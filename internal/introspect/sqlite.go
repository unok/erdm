package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// sqliteIntrospector は SQLite 用のスキーマ取得アダプタ。
//
// 担当範囲（design.md §"コンポーネントとインターフェース" / 要件 2.3, 3.1,
// 3.2, 4.x, 5.x, 6.x, 7.x, 8.3）:
//   - sqlite_master / pragma_table_info / pragma_foreign_key_list /
//     pragma_index_list / pragma_index_info を `SELECT` 経由で発行する
//     （要件 10.1）。
//   - SQLite には PostgreSQL ／ MySQL 相当の READ ONLY トランザクション文が
//     ない。本ドライバは `BeginTx` を開始せず、SELECT ／ PRAGMA のみを発行する
//     運用ルールで読み取り専用性を担保する（要件 10.1 / 10.3）。接続の
//     クローズは呼び出し側（タスク 8.1 で配線される `Introspect`）の責務。
//   - sqlite_master.type='table' で実テーブルのみを抽出し、システムテーブル
//     （sqlite_*）と一時テーブルは除外する（要件 3.1 / 3.2）。
//   - sqlite_master.sql から CREATE TABLE 原文を取得し、行末コメント抽出に
//     利用する（要件 8.3）。抽出不能な場合は空文字列とし、後段の物理名
//     フォールバックに委ねる（要件 8.6）。
type sqliteIntrospector struct {
	db *sql.DB
}

// newSQLiteIntrospector は接続済み *sql.DB を受け取ってイントロスペクタを
// 構築する。SQLite は接続先がファイル単位なのでスキーマ指定は受け付けない。
func newSQLiteIntrospector(db *sql.DB) *sqliteIntrospector {
	return &sqliteIntrospector{db: db}
}

// fetch は対象 SQLite ファイルからテーブル・カラム・主キー・外部キー・
// インデックスを取得し、内部 DTO 列を返す。失敗時はそのままエラーを
// 伝播する（要件 11.2）。
//
// パイプライン（plan §"全体構造"）:
//  1. 接続疎通確認
//  2. テーブル一覧 + CREATE TABLE 原文
//  3. 各テーブル: カラム情報（自動 ROWID 連番抑止）／外部キー／補助インデックス
//     + UNIQUE 制約由来の単一カラム UNIQUE
//  4. CREATE TABLE 原文からのカラム行末コメント抽出
//  5. 単一カラム UNIQUE インデックスを `rawColumn.IsUnique` に補完
//  6. 単一カラム FK の `SourceUnique` を `rawColumn.IsUnique` から導出
func (s *sqliteIntrospector) fetch(ctx context.Context) ([]rawTable, error) {
	if err := s.db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("introspect/sqlite: ping: %w", err)
	}
	tables, ddls, err := selectSQLiteTables(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, nil
	}
	return s.assemble(ctx, tables, ddls)
}

// assemble は取得済みテーブル名列に対して残りのメタデータ取得段階をテーブル単位
// ループで実行し、rawTable 列を組み立てる。fetch の段階を 30 行以内に収めるための
// 分離。SQLite の PRAGMA テーブル関数は単一テーブル名引数しか取らないため、
// PG ／ MySQL のような「スキーマ全体を 1 SELECT で取得」は採用しない。
func (s *sqliteIntrospector) assemble(ctx context.Context, tables []string, ddls map[string]string) ([]rawTable, error) {
	out := make([]rawTable, 0, len(tables))
	for _, name := range tables {
		t, err := s.assembleTable(ctx, name, ddls[name])
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// assembleTable は単一テーブル分の取得・組み立てを担うヘルパ。
func (s *sqliteIntrospector) assembleTable(ctx context.Context, name, ddl string) (rawTable, error) {
	cols, pk, err := selectSQLiteColumns(ctx, s.db, name)
	if err != nil {
		return rawTable{}, err
	}
	fks, err := selectSQLiteForeignKeys(ctx, s.db, name)
	if err != nil {
		return rawTable{}, err
	}
	indexes, uniqueCols, err := selectSQLiteIndexes(ctx, s.db, name)
	if err != nil {
		return rawTable{}, err
	}
	comments := extractSQLiteColumnComments(ddl, columnNames(cols))
	return buildSQLiteRawTable(name, cols, comments, uniqueCols, pk, fks, indexes), nil
}

// buildSQLiteRawTable は単一テーブル分の取得結果を rawTable に組み立てる純粋関数。
// カラムコメント反映 → UNIQUE 制約由来の単一カラム UNIQUE 反映 → 補助インデックス
// 由来の単一カラム UNIQUE 補完 → FK SourceUnique 補完まで完了させる。
//
// SQLite はテーブルコメント機構を持たない（CREATE TABLE 直前の行コメントは
// `sqlite_master.sql` に保存されないため抽出対象外）。Comment は空文字列とし、
// 物理名フォールバックに委ねる（要件 8.6）。
func buildSQLiteRawTable(name string, cols []rawColumn, comments map[string]string, uniqueCols, pk []string, fks []rawForeignKey, indexes []rawIndex) rawTable {
	for i := range cols {
		if c, ok := comments[cols[i].Name]; ok {
			cols[i].Comment = c
		}
	}
	for _, uc := range uniqueCols {
		markColumnUnique(cols, uc)
	}
	t := rawTable{
		Name:        name,
		Comment:     "",
		Columns:     cols,
		PrimaryKey:  pk,
		ForeignKeys: fks,
		Indexes:     indexes,
	}
	applySingleColumnUnique(&t)
	applyFKSourceUnique(&t)
	return t
}

// columnNames は rawColumn 列から物理名のスライスを返す純粋ヘルパ。
// extractSQLiteColumnComments の `knownColumns` 引数を組み立てるために使う。
func columnNames(cols []rawColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
	}
	return out
}

// normalizeSQLiteAutoIncrement は SQLite の自動 ROWID 連番列（`INTEGER PRIMARY
// KEY` 単一主キー）を検出し、デフォルト値を空にクリアする純粋関数（要件 4.7）。
//
// 検出条件:
//   - 単一主キー（isSinglePK）かつ当該カラムが PK 構成（isPK）
//   - 型が空（""）または "INTEGER"（大小文字無視）
//
// 該当時の動作:
//   - 型が空のときは "INTEGER" に補正する。型が指定済みのときは元の表記を保持
//     （大小文字を含む、要件 4.2 の「取得した型表記を保持」）。
//   - Default を空文字列にクリアする（自動 ROWID は呼び出し時に rowid が割り
//     当てられるため、保持しても意味のある値にならない）。
//
// 該当しない場合は (typeStr, defaultIn) をそのまま返す。
func normalizeSQLiteAutoIncrement(typeStr, defaultIn string, isPK, isSinglePK bool) (typeOut, defaultOut string) {
	if !isPK || !isSinglePK {
		return typeStr, defaultIn
	}
	upper := strings.ToUpper(strings.TrimSpace(typeStr))
	if upper != "" && upper != "INTEGER" {
		return typeStr, defaultIn
	}
	if typeStr == "" {
		return "INTEGER", ""
	}
	return typeStr, ""
}
