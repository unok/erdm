package introspect

import (
	"context"
	"database/sql"
	"fmt"
)

// postgres_queries.go は PostgreSQL 用の SELECT 文と各クエリ実行ヘルパを集約する。
//
// 集約方針（ナレッジ「操作の一覧性」）:
//   - 関数内文字列リテラルに散らさず、関数外 const 定数として 1 ファイルに集約する。
//     SQL 改変・レビュー時に PostgreSQL が発行するクエリ全量を 1 箇所で見渡せる。
//   - 各クエリ実行ヘルパは `*sql.Tx`（READ ONLY）を受け取り、テーブル名キーの
//     map ／配列など、`postgresIntrospector.fetch` のオーケストレータが扱いやすい
//     形に整形して返す。
//   - 失敗時は `fmt.Errorf("introspect/postgres: <段階>: %w", err)` の規約で
//     ラップする（呼び出し側が段階を一目で識別できるようにする）。

// sqlSelectPGTables は対象スキーマに属する実テーブル名を取得する SELECT 文。
//
//   - `table_type = 'BASE TABLE'` でビュー／マテビュー／外部テーブル／一時表を除外
//     （要件 3.2）。
//   - スキーマ系（`pg_catalog` ／ `information_schema`）は呼び出し側が
//     `Options.Schema` ／ 既定 `public` を渡す制約で自然に除外される（要件 3.1）。
//   - 取得順序は `information_schema.tables` の規定順をそのまま採用する（要件 3.6）。
const sqlSelectPGTables = `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = $1
  AND table_type = 'BASE TABLE'
`

// sqlSelectPGTableComments は対象スキーマの実テーブルおよびパーティション親に
// 紐づくテーブルコメントを `pg_description` から取得する SELECT 文（要件 8.1）。
//
// `relkind IN ('r', 'p')` で通常テーブルとパーティション親をカバーする。
const sqlSelectPGTableComments = `
SELECT cls.relname, COALESCE(d.description, '')
FROM pg_class cls
JOIN pg_namespace ns ON ns.oid = cls.relnamespace
LEFT JOIN pg_description d ON d.objoid = cls.oid AND d.objsubid = 0
WHERE ns.nspname = $1
  AND cls.relkind IN ('r', 'p')
`

// sqlSelectPGColumns は対象スキーマの全カラムを宣言順で取得する SELECT 文。
//
//   - `ORDER BY table_name, ordinal_position` でテーブル別の宣言順を保証する
//     （要件 4.1）。
//   - `is_nullable` は 'YES' / 'NO'（要件 4.3）。
//   - `column_default` の NULL は SQL 側で空文字列に正規化する（要件 4.5 / 4.7）。
//   - `udt_name` は `data_type='USER-DEFINED'`（enum 等のユーザ定義型）の場合の
//     表示名解決に使用する。
//   - 末尾 4 カラムは型の修飾子（`varchar(N)` / `numeric(p,s)` / `timestamp(N)` 等）
//     を組み立てるための材料。配列カラムは `information_schema.columns` 側では
//     これらが NULL になるため、`information_schema.element_types` を `dtd_identifier`
//     で LEFT JOIN し、要素型の修飾子を COALESCE で優先する（`numeric(10,2)[]`
//     のような配列要素の精度復元のため）。
const sqlSelectPGColumns = `
SELECT
    c.table_name,
    c.column_name,
    c.ordinal_position,
    c.data_type,
    c.udt_name,
    c.is_nullable,
    COALESCE(c.column_default, ''),
    COALESCE(et.character_maximum_length, c.character_maximum_length),
    COALESCE(et.numeric_precision, c.numeric_precision),
    COALESCE(et.numeric_scale, c.numeric_scale),
    COALESCE(et.datetime_precision, c.datetime_precision)
FROM information_schema.columns c
LEFT JOIN information_schema.element_types et
    ON et.object_catalog = c.table_catalog
   AND et.object_schema = c.table_schema
   AND et.object_name = c.table_name
   AND et.object_type = 'TABLE'
   AND et.collection_type_identifier = c.dtd_identifier
WHERE c.table_schema = $1
ORDER BY c.table_name, c.ordinal_position
`

// sqlSelectPGColumnComments はカラムコメントを `pg_description` から取得する
// SELECT 文（要件 8.1）。
//
// `attr.attnum > 0 AND NOT attr.attisdropped` でシステム列および削除済み列を除外。
const sqlSelectPGColumnComments = `
SELECT cls.relname, attr.attname, COALESCE(d.description, '')
FROM pg_class cls
JOIN pg_namespace ns ON ns.oid = cls.relnamespace
JOIN pg_attribute attr ON attr.attrelid = cls.oid AND attr.attnum > 0 AND NOT attr.attisdropped
LEFT JOIN pg_description d ON d.objoid = cls.oid AND d.objsubid = attr.attnum
WHERE ns.nspname = $1
  AND cls.relkind IN ('r', 'p')
`

// sqlSelectPGPrimaryKeys は PRIMARY KEY 制約の構成カラムを宣言順で取得する
// SELECT 文（要件 5.1 / 5.2）。
const sqlSelectPGPrimaryKeys = `
SELECT tc.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_schema = tc.constraint_schema
 AND kcu.constraint_name = tc.constraint_name
 AND kcu.table_schema = tc.table_schema
 AND kcu.table_name = tc.table_name
WHERE tc.table_schema = $1
  AND tc.constraint_type = 'PRIMARY KEY'
ORDER BY tc.table_name, tc.constraint_name, kcu.ordinal_position
`

// sqlSelectPGUniqueConstraints は UNIQUE 制約の構成カラムを取得する SELECT 文。
//
// 単一カラム UNIQUE 制約は `rawColumn.IsUnique` に反映される（要件 4.4）。
// 複合 UNIQUE 制約はカラム単位の UNIQUE 性に影響しないため、後段の Go コードで
// `constraint_name` ごとにグルーピングして単一カラムのみを抽出する。
const sqlSelectPGUniqueConstraints = `
SELECT tc.table_name, tc.constraint_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_schema = tc.constraint_schema
 AND kcu.constraint_name = tc.constraint_name
 AND kcu.table_schema = tc.table_schema
 AND kcu.table_name = tc.table_name
WHERE tc.table_schema = $1
  AND tc.constraint_type = 'UNIQUE'
ORDER BY tc.table_name, tc.constraint_name, kcu.ordinal_position
`

// sqlSelectPGForeignKeys は FOREIGN KEY 制約と参照先テーブルを取得する SELECT 文。
//
//   - `pg_constraint.conkey` 配列を `unnest WITH ORDINALITY` で展開し、複合 FK の
//     構成カラム順序を保持する（要件 6.4）。
//   - 参照先テーブル名は `con.confrelid` から `pg_class` を引いて解決する。
//     参照先がスコープ外スキーマでも、テーブル物理名のみを返し、判定は呼び出し側
//     （タスク 7.4 buildSchema）に委ねる（要件 6.5）。
const sqlSelectPGForeignKeys = `
SELECT
    src.relname AS source_table,
    con.conname AS constraint_name,
    src_attr.attname AS source_column,
    ord.n AS position,
    ref.relname AS target_table
FROM pg_constraint con
JOIN pg_class src ON src.oid = con.conrelid
JOIN pg_namespace ns ON ns.oid = src.relnamespace
JOIN pg_class ref ON ref.oid = con.confrelid
JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS ord(attnum, n) ON true
JOIN pg_attribute src_attr ON src_attr.attrelid = src.oid AND src_attr.attnum = ord.attnum
WHERE con.contype = 'f'
  AND ns.nspname = $1
ORDER BY src.relname, con.conname, ord.n
`

// sqlSelectPGIndexes は補助インデックスを取得する SELECT 文（要件 7.1 / 7.2 / 7.3）。
//
//   - `i.indisprimary = false` で PK インデックスを除外。
//   - `NOT EXISTS (... pg_constraint contype IN ('p','u'))` で PK／UQ 制約由来の
//     インデックスを除外。CREATE UNIQUE INDEX で直接作られた UNIQUE インデックスは
//     `pg_constraint` に対応行を持たないため、本フィルタを通り抜けて保持される。
//   - `unnest(i.indkey) WITH ORDINALITY` で構成カラムの順序を保持。
//   - 式インデックスのカラム（attnum=0）は `pg_attribute` JOIN で自然に除外される。
const sqlSelectPGIndexes = `
SELECT
    tbl.relname AS table_name,
    idx.relname AS index_name,
    i.indisunique AS is_unique,
    attr.attname AS column_name,
    ord.n AS position
FROM pg_index i
JOIN pg_class tbl ON tbl.oid = i.indrelid
JOIN pg_class idx ON idx.oid = i.indexrelid
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS ord(attnum, n) ON true
JOIN pg_attribute attr
  ON attr.attrelid = tbl.oid
 AND attr.attnum = ord.attnum
 AND attr.attnum > 0
 AND NOT attr.attisdropped
WHERE ns.nspname = $1
  AND i.indisprimary = false
  AND NOT EXISTS (
      SELECT 1 FROM pg_constraint c
      WHERE c.conindid = i.indexrelid
        AND c.contype IN ('p', 'u')
  )
ORDER BY tbl.relname, idx.relname, ord.n
`

// pgColumnRow は selectPGColumns が SQL 結果を Go 側で扱うための一時行。
// 取得後に normalizePGSerial／USER-DEFINED 解決／型修飾子付与を経て rawColumn に変換する。
//
// 末尾の 4 つの NullInt64 は配列カラムの場合は要素型側の値（element_types JOIN
// 由来）、非配列カラムでは columns 自身の値が入る（SQL 側で COALESCE 済）。
type pgColumnRow struct {
	Table         string
	Name          string
	Position      int
	DataType      string
	UDTName       string
	IsNullable    string
	Default       string
	CharMaxLength sql.NullInt64
	NumPrecision sql.NullInt64
	NumScale     sql.NullInt64
	DTPrecision  sql.NullInt64
}

// pgFKRow は selectPGForeignKeys の 1 行を表す。同一 constraintName のカラムを
// position 昇順で集めて 1 件の rawForeignKey に組み立てる。
type pgFKRow struct {
	SourceTable    string
	ConstraintName string
	SourceColumn   string
	Position       int
	TargetTable    string
}

// pgIndexRow は selectPGIndexes の 1 行を表す。同一 (table, index) のカラムを
// position 昇順で集めて 1 件の rawIndex に組み立てる。
type pgIndexRow struct {
	Table     string
	IndexName string
	IsUnique  bool
	Column    string
	Position  int
}

// pgUniqueRow は selectPGSingleColumnUniques の 1 行を表す。
// 同一 (table, constraint) のカラム数が 1 のときだけ単一カラム UNIQUE と判定する。
type pgUniqueRow struct {
	Table          string
	ConstraintName string
	Column         string
}

// selectPGTables は対象スキーマに属する実テーブル名を返す（要件 3.x）。
func selectPGTables(ctx context.Context, tx *sql.Tx, schema string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGTables, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select tables: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, 32)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select tables: scan: %w", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select tables: rows: %w", err)
	}
	return out, nil
}

// selectPGTableComments は table_name -> comment の map を返す（要件 8.1）。
func selectPGTableComments(ctx context.Context, tx *sql.Tx, schema string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGTableComments, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select table comments: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select table comments: scan: %w", err)
		}
		out[name] = comment
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select table comments: rows: %w", err)
	}
	return out, nil
}

// selectPGColumns はテーブル名キーで宣言順カラム列を返す（要件 4.x / 8.1）。
//
// 自動連番列（`nextval(...)` + integer 系）の型表記正規化とデフォルト値クリア、
// および USER-DEFINED 型の表示名フォールバックは本関数で適用する。
// 単一カラム UNIQUE 性は別段で `applySingleColumnUnique`／単一カラム UNIQUE 制約
// のマージで補完される（取得段階では UNIQUE 性は未確定）。
func selectPGColumns(ctx context.Context, tx *sql.Tx, schema string) (map[string][]rawColumn, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGColumns, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select columns: %w", err)
	}
	defer rows.Close()
	out := map[string][]rawColumn{}
	for rows.Next() {
		var r pgColumnRow
		if err := rows.Scan(&r.Table, &r.Name, &r.Position, &r.DataType, &r.UDTName, &r.IsNullable, &r.Default,
			&r.CharMaxLength, &r.NumPrecision, &r.NumScale, &r.DTPrecision); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select columns: scan: %w", err)
		}
		typ, defOut := normalizePGSerial(r.DataType, r.Default)
		typ = resolvePGType(typ, r.UDTName)
		typ = applyPGTypeModifier(typ, r.CharMaxLength, r.NumPrecision, r.NumScale, r.DTPrecision)
		out[r.Table] = append(out[r.Table], rawColumn{
			Name:    r.Name,
			Type:    typ,
			NotNull: r.IsNullable == "NO",
			Default: defOut,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select columns: rows: %w", err)
	}
	return out, nil
}

// selectPGColumnComments は (table, column) -> comment の map を返す（要件 8.1）。
func selectPGColumnComments(ctx context.Context, tx *sql.Tx, schema string) (map[tableColumnKey]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGColumnComments, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select column comments: %w", err)
	}
	defer rows.Close()
	out := map[tableColumnKey]string{}
	for rows.Next() {
		var tbl, col, comment string
		if err := rows.Scan(&tbl, &col, &comment); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select column comments: scan: %w", err)
		}
		out[tableColumnKey{Table: tbl, Column: col}] = comment
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select column comments: rows: %w", err)
	}
	return out, nil
}

// selectPGPrimaryKeys はテーブル名キーで PK 構成カラム列を返す（要件 5.x）。
func selectPGPrimaryKeys(ctx context.Context, tx *sql.Tx, schema string) (map[string][]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGPrimaryKeys, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select primary keys: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select primary keys: scan: %w", err)
		}
		out[tbl] = append(out[tbl], col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select primary keys: rows: %w", err)
	}
	return out, nil
}

// selectPGSingleColumnUniques はテーブル名キーで「単一カラム UNIQUE 制約の対象
// カラム名」配列を返す（要件 4.4）。複合 UNIQUE 制約はここでは除外する。
func selectPGSingleColumnUniques(ctx context.Context, tx *sql.Tx, schema string) (map[string][]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGUniqueConstraints, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select unique constraints: %w", err)
	}
	defer rows.Close()
	type key struct {
		Table      string
		Constraint string
	}
	grouped := map[key][]string{}
	keysOrder := make([]key, 0, 16)
	for rows.Next() {
		var r pgUniqueRow
		if err := rows.Scan(&r.Table, &r.ConstraintName, &r.Column); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select unique constraints: scan: %w", err)
		}
		k := key{Table: r.Table, Constraint: r.ConstraintName}
		if _, exists := grouped[k]; !exists {
			keysOrder = append(keysOrder, k)
		}
		grouped[k] = append(grouped[k], r.Column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select unique constraints: rows: %w", err)
	}
	out := map[string][]string{}
	for _, k := range keysOrder {
		cols := grouped[k]
		if len(cols) != 1 {
			continue
		}
		out[k.Table] = append(out[k.Table], cols[0])
	}
	return out, nil
}

// selectPGForeignKeys はテーブル名キーで FK 列を返す（要件 6.x）。
//
// 同一 constraint_name のカラムを position 昇順で 1 件の rawForeignKey に集約する。
// 単一カラム FK の SourceUnique は `applyFKSourceUnique` で別段に補完するため、
// 取得段階では false とする。
func selectPGForeignKeys(ctx context.Context, tx *sql.Tx, schema string) (map[string][]rawForeignKey, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGForeignKeys, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select foreign keys: %w", err)
	}
	defer rows.Close()
	type key struct {
		Table      string
		Constraint string
	}
	type entry struct {
		TargetTable string
		Columns     []string
	}
	grouped := map[key]*entry{}
	tableOrder := make([]string, 0, 16)
	tableConstraints := map[string][]key{}
	for rows.Next() {
		var r pgFKRow
		if err := rows.Scan(&r.SourceTable, &r.ConstraintName, &r.SourceColumn, &r.Position, &r.TargetTable); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select foreign keys: scan: %w", err)
		}
		k := key{Table: r.SourceTable, Constraint: r.ConstraintName}
		e, exists := grouped[k]
		if !exists {
			e = &entry{TargetTable: r.TargetTable}
			grouped[k] = e
			if _, seen := tableConstraints[r.SourceTable]; !seen {
				tableOrder = append(tableOrder, r.SourceTable)
			}
			tableConstraints[r.SourceTable] = append(tableConstraints[r.SourceTable], k)
		}
		e.Columns = append(e.Columns, r.SourceColumn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select foreign keys: rows: %w", err)
	}
	out := map[string][]rawForeignKey{}
	for _, tbl := range tableOrder {
		for _, k := range tableConstraints[tbl] {
			e := grouped[k]
			out[tbl] = append(out[tbl], rawForeignKey{
				SourceColumns: e.Columns,
				TargetTable:   e.TargetTable,
			})
		}
	}
	return out, nil
}

// selectPGIndexes はテーブル名キーで補助インデックス列を返す（要件 7.x）。
//
// 同一 index_name のカラムを position 昇順で 1 件の rawIndex に集約する。
func selectPGIndexes(ctx context.Context, tx *sql.Tx, schema string) (map[string][]rawIndex, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectPGIndexes, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/postgres: select indexes: %w", err)
	}
	defer rows.Close()
	type key struct {
		Table string
		Index string
	}
	type entry struct {
		IsUnique bool
		Columns  []string
	}
	grouped := map[key]*entry{}
	tableOrder := make([]string, 0, 16)
	tableIndexes := map[string][]key{}
	for rows.Next() {
		var r pgIndexRow
		if err := rows.Scan(&r.Table, &r.IndexName, &r.IsUnique, &r.Column, &r.Position); err != nil {
			return nil, fmt.Errorf("introspect/postgres: select indexes: scan: %w", err)
		}
		k := key{Table: r.Table, Index: r.IndexName}
		e, exists := grouped[k]
		if !exists {
			e = &entry{IsUnique: r.IsUnique}
			grouped[k] = e
			if _, seen := tableIndexes[r.Table]; !seen {
				tableOrder = append(tableOrder, r.Table)
			}
			tableIndexes[r.Table] = append(tableIndexes[r.Table], k)
		}
		e.Columns = append(e.Columns, r.Column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/postgres: select indexes: rows: %w", err)
	}
	out := map[string][]rawIndex{}
	for _, tbl := range tableOrder {
		for _, k := range tableIndexes[tbl] {
			e := grouped[k]
			out[tbl] = append(out[tbl], rawIndex{
				Name:     k.Index,
				Columns:  e.Columns,
				IsUnique: e.IsUnique,
			})
		}
	}
	return out, nil
}
