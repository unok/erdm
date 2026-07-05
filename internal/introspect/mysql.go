package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// mysqlIntrospector は MySQL 用のスキーマ取得アダプタ。
//
// 担当範囲（design.md §"コンポーネントとインターフェース" / 要件 2.2, 3.1,
// 3.3, 4.x, 5.x, 6.x, 7.x, 8.2）:
//   - information_schema を読み取り対象とし、SELECT のみを発行する（要件 10.1）。
//   - 起点は READ ONLY トランザクションで、処理完了時または異常終了時のいずれ
//     でも tx.Rollback を必ず呼ぶ（要件 10.2 / 10.3）。
//   - システムスキーマ群（mysql, information_schema, performance_schema, sys）
//     とビュー・一時テーブルは WHERE 条件で除外する（要件 3.1 / 3.2）。
//   - TABLE_COMMENT / COLUMN_COMMENT を取得する（要件 8.2）。
//
// READ ONLY TX の流儀（plan §決定ログ）:
//
// PostgreSQL では `BeginTx(ReadOnly:true)` ＋ `SET TRANSACTION READ ONLY` の
// 二重保証を採用したが、MySQL では go-sql-driver/mysql の仕様により
// `BeginTx(ReadOnly:true)` のみで `START TRANSACTION READ ONLY` が送信される。
// 追加で `SET TRANSACTION READ ONLY` を発行すると二重発行エラーになるため、
// 本ドライバでは `BeginTx(ReadOnly:true)` のみで READ ONLY を担保する。
type mysqlIntrospector struct {
	db     *sql.DB
	schema string
}

// newMySQLIntrospector は接続済み *sql.DB と対象スキーマ名（DB 名）を受け取って
// イントロスペクタを構築する。スキーマ名が空文字の場合は接続先 DB を既定
// 対象とする（要件 3.3）。空文字の解決自体は接続後にしか行えないため、
// コンストラクタは入力を素通しする責務に留める（実解決は fetch 内）。
func newMySQLIntrospector(db *sql.DB, schema string) *mysqlIntrospector {
	return &mysqlIntrospector{db: db, schema: schema}
}

// fetch は対象 DB からテーブル・カラム・主キー・外部キー・インデックスを
// 取得し、内部 DTO 列を返す。失敗時はそのままエラーを伝播する（要件 11.2）。
//
// パイプライン:
//  1. 接続疎通確認 → READ ONLY TX 開始
//  2. 接続先 DB 名解決（schema 未指定時は SELECT DATABASE()）
//  3. テーブル一覧 + テーブルコメント
//  4. カラム情報（AUTO_INCREMENT 抑止を normalizeMySQLAutoIncrement で適用）
//  5. 主キー / 外部キー / 補助インデックス
//  6. 単一カラム UNIQUE 制約の `rawColumn.IsUnique` 反映
//  7. 補助インデックス由来の単一カラム UNIQUE を `applySingleColumnUnique` で補完
//  8. 単一カラム FK の `SourceUnique` を `applyFKSourceUnique` で導出
func (m *mysqlIntrospector) fetch(ctx context.Context) ([]rawTable, error) {
	if err := m.db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("introspect/mysql: ping: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("introspect/mysql: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	schema, err := resolveMySQLSchema(ctx, tx, m.schema)
	if err != nil {
		return nil, err
	}
	tables, tableComments, err := selectMySQLTables(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, nil
	}
	return m.assemble(ctx, tx, schema, tables, tableComments)
}

// assemble は取得済みテーブル名列に対して残りのメタデータ取得段階を順次実行し、
// rawTable 列を組み立てる。fetch の段階を 30 行以内に収めるための分離。
func (m *mysqlIntrospector) assemble(ctx context.Context, tx *sql.Tx, schema string, tables []string, tableComments map[string]string) ([]rawTable, error) {
	columns, colComments, err := selectMySQLColumns(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	uniqueCols, err := selectMySQLSingleColumnUniques(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	pks, err := selectMySQLPrimaryKeys(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	fks, err := selectMySQLForeignKeys(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	indexes, err := selectMySQLIndexes(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	out := make([]rawTable, 0, len(tables))
	for _, name := range tables {
		t := buildMySQLRawTable(name, tableComments[name], columns[name], colComments, uniqueCols[name], pks[name], fks[name], indexes[name])
		out = append(out, t)
	}
	return out, nil
}

// buildMySQLRawTable は単一テーブル分の取得結果を rawTable に組み立てる純粋関数。
// 単一カラム UNIQUE の補完および FK SourceUnique の補完まで完了させる。
func buildMySQLRawTable(name, comment string, cols []rawColumn, colComments map[tableColumnKey]string, uniqueCols, pk []string, fks []rawForeignKey, indexes []rawIndex) rawTable {
	for i := range cols {
		cols[i].Comment = colComments[tableColumnKey{Table: name, Column: cols[i].Name}]
	}
	for _, uc := range uniqueCols {
		markColumnUnique(cols, uc)
	}
	t := rawTable{
		Name:        name,
		Comment:     comment,
		Columns:     cols,
		PrimaryKey:  pk,
		ForeignKeys: fks,
		Indexes:     indexes,
	}
	applySingleColumnUnique(&t)
	applyFKSourceUnique(&t)
	return t
}

// normalizeMySQLAutoIncrement は MySQL の AUTO_INCREMENT 列を検出し、
// デフォルト値を空にクリアする純粋関数（要件 4.7）。
//
// 検出条件:
//   - EXTRA に "auto_increment"（大文字小文字無視）が含まれる
//
// AUTO_INCREMENT 列は MySQL がシーケンスの次値を実行時に決めるため、
// 出力に Default を残すと意味の乱れた erdm 表現になる。検出された場合は
// Default を空文字列にクリアし、それ以外は入力をそのまま返す。
func normalizeMySQLAutoIncrement(extra, columnDefault string) string {
	if strings.Contains(strings.ToLower(extra), "auto_increment") {
		return ""
	}
	return columnDefault
}
