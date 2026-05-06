package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// postgresIntrospector は PostgreSQL 用のスキーマ取得アダプタ。
//
// 担当範囲（design.md §"コンポーネントとインターフェース" / 要件 2.1, 3.1,
// 3.3, 3.4, 4.x, 5.x, 6.x, 7.x, 8.1）:
//   - database/sql 抽象を介して SELECT 文のみを発行する（要件 10.1）。
//   - 起点は READ ONLY トランザクション（`sql.TxOptions{ReadOnly: true}` ＋
//     `SET TRANSACTION READ ONLY` の二重保証）で、処理完了時または異常終了時の
//     いずれでも `tx.Rollback` を必ず呼ぶ（要件 10.2 / 10.3）。
//   - システムスキーマ・ビュー・マテリアライズドビュー・外部テーブル・一時
//     テーブルは WHERE 条件で除外する（要件 3.1 / 3.2）。
//   - pg_description JOIN でテーブル／カラムコメントを取得する（要件 8.1）。
//
// 本層では DSN 文字列を一切扱わない。`maskDSN` 経由のエラー文言生成は
// `Introspect`（タスク 8.1 で配線）の責務とする（要件 10.4）。
type postgresIntrospector struct {
	db     *sql.DB
	schema string
}

// tableColumnKey はカラムコメントを (table, column) で索引するための合成キー。
// map の key 用途で外部に漏れる必要はない package private 型。
type tableColumnKey struct {
	Table  string
	Column string
}

// newPostgresIntrospector は接続済み *sql.DB と対象スキーマ名を受け取って
// イントロスペクタを構築する。スキーマ名が空文字の場合は public を採用する
// （要件 3.4）。
func newPostgresIntrospector(db *sql.DB, schema string) *postgresIntrospector {
	if schema == "" {
		schema = "public"
	}
	return &postgresIntrospector{db: db, schema: schema}
}

// fetch は対象スキーマからテーブル・カラム・主キー・外部キー・インデックスを
// 取得し、内部 DTO 列を返す。失敗時はそのままエラーを伝播する（要件 11.2）。
//
// パイプライン（plan §"全体構造"）:
//  1. 接続疎通確認 → READ ONLY TX 開始 → `SET TRANSACTION READ ONLY` の二重保証
//  2. テーブル一覧 → テーブルコメント
//  3. カラム情報 + 自動連番正規化 → カラムコメント
//  4. 単一カラム UNIQUE 制約マージ
//  5. 主キー / 外部キー / 補助インデックス
//  6. 単一カラム UNIQUE インデックスを `rawColumn.IsUnique` に補完
//  7. 単一カラム FK の `SourceUnique` を `rawColumn.IsUnique` から導出
func (p *postgresIntrospector) fetch(ctx context.Context) ([]rawTable, error) {
	if err := p.db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("introspect/postgres: ping: %w", err)
	}
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SET TRANSACTION READ ONLY"); err != nil {
		return nil, fmt.Errorf("introspect/postgres: set read only: %w", err)
	}
	tables, err := selectPGTables(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, nil
	}
	return p.assemble(ctx, tx, tables)
}

// assemble は取得済みテーブル名列に対して残りのメタデータ取得段階を順次実行し、
// rawTable 列を組み立てる。fetch の段階を 30 行以内で収めるための分離。
func (p *postgresIntrospector) assemble(ctx context.Context, tx *sql.Tx, tables []string) ([]rawTable, error) {
	tableComments, err := selectPGTableComments(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	columns, err := selectPGColumns(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	colComments, err := selectPGColumnComments(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	uniqueCols, err := selectPGSingleColumnUniques(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	pks, err := selectPGPrimaryKeys(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	fks, err := selectPGForeignKeys(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	indexes, err := selectPGIndexes(ctx, tx, p.schema)
	if err != nil {
		return nil, err
	}
	out := make([]rawTable, 0, len(tables))
	for _, name := range tables {
		t := buildPGRawTable(name, tableComments[name], columns[name], colComments, uniqueCols[name], pks[name], fks[name], indexes[name])
		out = append(out, t)
	}
	return out, nil
}

// buildPGRawTable は単一テーブル分の取得結果を rawTable に組み立てる純粋関数。
// 単一カラム UNIQUE の補完および FK SourceUnique の補完まで完了させる。
func buildPGRawTable(name, comment string, cols []rawColumn, colComments map[tableColumnKey]string, uniqueCols, pk []string, fks []rawForeignKey, indexes []rawIndex) rawTable {
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

// normalizePGSerial は PostgreSQL の自動連番列を検出し、対応する serial 系の
// 型表記とデフォルト値クリアを返す純粋関数（要件 4.7）。
//
// 検出条件:
//   - column_default が `nextval(` で始まる
//   - data_type が integer 系（smallint / integer / bigint）
//
// 該当しない場合は (dataType, columnDefault) をそのまま返す。
func normalizePGSerial(dataType, columnDefault string) (string, string) {
	if !strings.HasPrefix(columnDefault, "nextval(") {
		return dataType, columnDefault
	}
	switch dataType {
	case "smallint":
		return "smallserial", ""
	case "integer":
		return "serial", ""
	case "bigint":
		return "bigserial", ""
	}
	return dataType, columnDefault
}

// resolvePGType は PostgreSQL の `data_type` が `USER-DEFINED`（enum など）の
// ときに `udt_name` を表示名としてフォールバックする純粋関数。
// それ以外の場合は dataType をそのまま返す。
func resolvePGType(dataType, udtName string) string {
	if dataType == "USER-DEFINED" && udtName != "" {
		return udtName
	}
	return dataType
}

// markColumnUnique は対象カラムが見つかれば IsUnique=true を立てる純粋関数。
// 同一カラムに複数の UNIQUE 起源（制約／インデックス）がぶつかっても冪等。
func markColumnUnique(cols []rawColumn, columnName string) {
	for i := range cols {
		if cols[i].Name == columnName {
			cols[i].IsUnique = true
			return
		}
	}
}

// applySingleColumnUnique は補助インデックス由来の単一カラム UNIQUE を
// `rawColumn.IsUnique` に反映する純粋関数（要件 4.4）。
//
// 補助インデックス（PK／UQ 制約に紐づかないもの）は rawTable.Indexes に
// 残っており、その中で `IsUnique && len(Columns)==1` のものを対応カラムへ反映する。
// 複合 UNIQUE インデックス／非 UNIQUE インデックスは無視する。
func applySingleColumnUnique(t *rawTable) {
	for _, idx := range t.Indexes {
		if !idx.IsUnique || len(idx.Columns) != 1 {
			continue
		}
		markColumnUnique(t.Columns, idx.Columns[0])
	}
}

// applyFKSourceUnique は単一カラム FK の `SourceUnique` を
// `rawColumn.IsUnique` から導出する純粋関数（要件 6.3 のカーディナリティ判定の
// 前提条件）。複合 FK は対象外（要件 6.4 で先頭カラムにのみ関係を付与する判定は
// builder.go の責務）。
//
// UNIQUE と判定する条件は以下のいずれか:
//   - 当該カラムの `IsUnique` が true（UNIQUE 制約・単一カラム UNIQUE インデックス由来）
//   - 当該カラムが単一カラム主キー（PK は本質的に UNIQUE であり、FK 参照元が
//     単独 PK の場合は 1 対 1 関係になる。要件 6.3）
func applyFKSourceUnique(t *rawTable) {
	singlePK := len(t.PrimaryKey) == 1
	for fi := range t.ForeignKeys {
		fk := &t.ForeignKeys[fi]
		if len(fk.SourceColumns) != 1 {
			continue
		}
		src := fk.SourceColumns[0]
		if singlePK && t.PrimaryKey[0] == src {
			fk.SourceUnique = true
			continue
		}
		for _, c := range t.Columns {
			if c.Name == src && c.IsUnique {
				fk.SourceUnique = true
				break
			}
		}
	}
}
