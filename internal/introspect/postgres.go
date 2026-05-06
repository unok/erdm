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

// resolvePGType は PostgreSQL の `data_type` を `.erdm` の col_type 表記に
// 解決する純粋関数。
//
// 解決規則（PostgreSQL `information_schema.columns` の仕様準拠）:
//
//   - `data_type='USER-DEFINED'` の場合は `udt_name` をそのまま返す（enum 等）。
//     udt_name が空のときは `USER-DEFINED` を維持する（フォールバック先がない）。
//   - `data_type='ARRAY'` の場合は `udt_name`（`_int4` / `_varchar` 等の内部名）を
//     `pgArrayElementDisplay` で短縮表示名に正規化し、末尾に `[]` を付与して返す
//     （例: `_int4` → `integer[]`、`_varchar` → `varchar[]`、`_timestamp` →
//     `timestamp[]`、`_timestamptz` → `timestamptz[]`）。udt_name が空のときは
//     失われる情報を残すため `ARRAY` を返す（壊れた既存挙動の保持）。
//   - 上記以外は `data_type` を `pgDataTypeShortName` で短縮表示名へ正規化して
//     返す（`character varying` → `varchar`、`timestamp without time zone` →
//     `timestamp`、`timestamp with time zone` → `timestamptz` 等）。短縮対象に
//     ない型は data_type をそのまま返す。
func resolvePGType(dataType, udtName string) string {
	switch dataType {
	case "USER-DEFINED":
		if udtName == "" {
			return dataType
		}
		return udtName
	case "ARRAY":
		if udtName == "" {
			return dataType
		}
		return pgArrayElementDisplay(udtName) + "[]"
	default:
		return pgDataTypeShortName(dataType)
	}
}

// pgDataTypeShortName は PostgreSQL の `information_schema.columns.data_type`
// が返す冗長な SQL 標準名を、erdm 慣用の短縮表記へ正規化する純粋関数。
//
// 写像対象（要件: `.erdm` の可読性向上）:
//   - `character varying`        → `varchar`
//   - `character`                → `character`（短縮しない。`char` は予約語混乱を避ける）
//   - `timestamp without time zone` → `timestamp`
//   - `timestamp with time zone`    → `timestamptz`
//   - `time without time zone`      → `time`
//   - `time with time zone`         → `timetz`
//   - `bit varying`              → `bit varying`（短縮しない）
//
// 短縮対象に無い型はそのまま返す（既存出力との後方互換）。
func pgDataTypeShortName(dataType string) string {
	if mapped, ok := pgDataTypeAliases[dataType]; ok {
		return mapped
	}
	return dataType
}

// pgDataTypeAliases は data_type 文字列の冗長表記を短縮表記へ写す表。
// 配列要素側（pgInternalToDisplay）と語尾の整合を取るため、両表は同じ
// 短縮ルール（`varchar` / `timestamp` / `timestamptz` / `time` / `timetz`）を採用する。
var pgDataTypeAliases = map[string]string{
	"character varying":           "varchar",
	"timestamp without time zone": "timestamp",
	"timestamp with time zone":    "timestamptz",
	"time without time zone":      "time",
	"time with time zone":         "timetz",
}

// pgArrayElementDisplay は配列カラムの `udt_name`（PG 内部名、先頭 `_` 付き）を
// `data_type` 寄りの SQL 表示名に変換する純粋関数。
//
// 変換は以下の段階で行う:
//  1. 先頭の `_` を 1 文字だけ取り除く（PG は配列型を `_<element>` で命名する）
//  2. 既知の内部名は `data_type` 寄りの表示名へ写像する
//  3. 未知の内部名（独自 enum など）はそのまま返す
//
// 既知マップは PostgreSQL の基本ビルトイン型（数値・文字列・日時・boolean・
// バイナリ・json/jsonb・uuid・ネットワーク・bit・xml・money・tsvector/tsquery）
// を網羅する。これに含まれない型は `_<name>` の `_` のみ取り除いた素の名前で
// 出力する（例: ユーザー定義 enum の配列 `_mood` → `mood`）。
func pgArrayElementDisplay(udtName string) string {
	stripped := strings.TrimPrefix(udtName, "_")
	if mapped, ok := pgInternalToDisplay[stripped]; ok {
		return mapped
	}
	return stripped
}

// pgInternalToDisplay は PG 内部型名（`udt_name` の `_` 抜き）から
// 表示名への写像。erdm 慣用の短縮表記（`varchar` / `timestamp` /
// `timestamptz` / `time` / `timetz`）に揃え、非配列側の `pgDataTypeShortName`
// と語尾を一致させる。
//
// ここに無いキーは（独自 enum 等とみなして）そのまま返す方針。テスト網羅は
// `postgres_test.go` の TestPGArrayElementDisplay / TestResolvePGType にある。
var pgInternalToDisplay = map[string]string{
	"int2":        "smallint",
	"int4":        "integer",
	"int8":        "bigint",
	"float4":      "real",
	"float8":      "double precision",
	"bool":        "boolean",
	"varchar":     "varchar",
	"bpchar":      "character",
	"timetz":      "timetz",
	"timestamptz": "timestamptz",
	"timestamp":   "timestamp",
	"time":        "time",
	"varbit":      "bit varying",
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
