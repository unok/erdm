package introspect

import (
	"context"
	"database/sql"
	"fmt"
)

// mysql_queries.go は MySQL 用の SELECT 文と各クエリ実行ヘルパを集約する。
//
// 集約方針（ナレッジ「操作の一覧性」）:
//   - 関数内文字列リテラルに散らさず、関数外 const 定数として 1 ファイルに集約する。
//     SQL 改変・レビュー時に MySQL が発行するクエリ全量を 1 箇所で見渡せる。
//   - 各クエリ実行ヘルパは `*sql.Tx`（READ ONLY）を受け取り、テーブル名キーの
//     map ／配列など、`mysqlIntrospector.fetch` のオーケストレータが扱いやすい
//     形に整形して返す。
//   - 失敗時は `fmt.Errorf("introspect/mysql: <段階>: %w", err)` の規約で
//     ラップする（呼び出し側が段階を一目で識別できるようにする）。

// sqlSelectMySQLDatabase は接続先 DB 名（USE 句で選択中のスキーマ）を返す。
//
// 接続時に DB を未指定で接続した場合は NULL が返るため、Scan 後に
// nil チェックを行い、説明的エラーを返す（要件 3.3 の補完）。
const sqlSelectMySQLDatabase = `SELECT DATABASE()`

// sqlSelectMySQLTables は対象スキーマに属する実テーブル名とテーブルコメントを
// 取得する SELECT 文。
//
//   - `TABLE_TYPE = 'BASE TABLE'` でビュー・一時テーブルを除外（要件 3.2）。
//   - システムスキーマ群（mysql ／ information_schema ／ performance_schema ／
//     sys）は呼び出し側が `Options.Schema` または `SELECT DATABASE()` で
//     非システム DB を指定する制約で自然に除外される（要件 3.1）。
//   - 取得順序は `information_schema.tables` の規定順を採用する（要件 3.6）。
const sqlSelectMySQLTables = `
SELECT TABLE_NAME, COALESCE(TABLE_COMMENT, '')
FROM information_schema.tables
WHERE TABLE_SCHEMA = ?
  AND TABLE_TYPE = 'BASE TABLE'
`

// sqlSelectMySQLColumns は対象スキーマの全カラムを宣言順で取得する SELECT 文。
//
//   - `ORDER BY TABLE_NAME, ORDINAL_POSITION` でテーブル別の宣言順を保証する
//     （要件 4.1）。
//   - `IS_NULLABLE` は 'YES' / 'NO'（要件 4.3）。
//   - `COLUMN_DEFAULT` の NULL は SQL 側で空文字列に正規化する（要件 4.5）。
//   - `COLUMN_TYPE` を採用することで `int(11)` 等の長さ情報を保持する（要件 4.2）。
//   - `EXTRA` から AUTO_INCREMENT を検出する（要件 4.7、Go 側の純粋ヘルパで処理）。
//   - `COLUMN_COMMENT` でカラムコメントを取得する（要件 8.2）。
const sqlSelectMySQLColumns = `
SELECT
    TABLE_NAME,
    COLUMN_NAME,
    ORDINAL_POSITION,
    COLUMN_TYPE,
    IS_NULLABLE,
    COALESCE(COLUMN_DEFAULT, ''),
    COALESCE(EXTRA, ''),
    COALESCE(COLUMN_COMMENT, '')
FROM information_schema.columns
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME, ORDINAL_POSITION
`

// sqlSelectMySQLPrimaryKeys は PRIMARY KEY 制約の構成カラムを宣言順で取得する
// SELECT 文（要件 5.1 / 5.2）。
//
// MySQL では主キー制約の名前は常に 'PRIMARY' に固定される。
const sqlSelectMySQLPrimaryKeys = `
SELECT TABLE_NAME, COLUMN_NAME
FROM information_schema.key_column_usage
WHERE TABLE_SCHEMA = ?
  AND CONSTRAINT_NAME = 'PRIMARY'
ORDER BY TABLE_NAME, ORDINAL_POSITION
`

// sqlSelectMySQLUniqueConstraints は UNIQUE 制約の構成カラムを取得する SELECT 文。
//
// 単一カラム UNIQUE 制約は `rawColumn.IsUnique` に反映される（要件 4.4）。
// 複合 UNIQUE 制約はカラム単位の UNIQUE 性に影響しないため、後段の Go コードで
// `CONSTRAINT_NAME` ごとにグルーピングして単一カラムのみを抽出する。
const sqlSelectMySQLUniqueConstraints = `
SELECT tc.TABLE_NAME, tc.CONSTRAINT_NAME, kcu.COLUMN_NAME
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA
 AND kcu.CONSTRAINT_NAME   = tc.CONSTRAINT_NAME
 AND kcu.TABLE_SCHEMA      = tc.TABLE_SCHEMA
 AND kcu.TABLE_NAME        = tc.TABLE_NAME
WHERE tc.TABLE_SCHEMA      = ?
  AND tc.CONSTRAINT_TYPE   = 'UNIQUE'
ORDER BY tc.TABLE_NAME, tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION
`

// sqlSelectMySQLForeignKeys は FOREIGN KEY 制約と参照先テーブルを取得する SELECT 文。
//
//   - `referential_constraints` JOIN `key_column_usage` で参照元構成カラムを
//     `ORDINAL_POSITION` 順で展開する（要件 6.4 の複合 FK 順序保持）。
//   - 参照先テーブル名は `referential_constraints.REFERENCED_TABLE_NAME`
//     から解決する。参照先がスコープ外スキーマでも、テーブル物理名のみを返し、
//     判定は呼び出し側（タスク 7.4 buildSchema）に委ねる（要件 6.5）。
const sqlSelectMySQLForeignKeys = `
SELECT
    kcu.TABLE_NAME      AS source_table,
    kcu.CONSTRAINT_NAME AS constraint_name,
    kcu.COLUMN_NAME     AS source_column,
    kcu.ORDINAL_POSITION AS position,
    kcu.REFERENCED_TABLE_NAME AS target_table
FROM information_schema.referential_constraints rc
JOIN information_schema.key_column_usage kcu
  ON kcu.CONSTRAINT_SCHEMA = rc.CONSTRAINT_SCHEMA
 AND kcu.CONSTRAINT_NAME   = rc.CONSTRAINT_NAME
 AND kcu.TABLE_SCHEMA      = rc.CONSTRAINT_SCHEMA
WHERE rc.CONSTRAINT_SCHEMA = ?
ORDER BY kcu.TABLE_NAME, kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION
`

// sqlSelectMySQLIndexes は補助インデックスを取得する SELECT 文（要件 7.1 / 7.2 / 7.3）。
//
//   - `INDEX_NAME != 'PRIMARY'` で PK インデックスを除外（要件 7.1）。
//   - `LEFT JOIN information_schema.table_constraints` で UNIQUE 制約起源の
//     インデックスを除外する（`CONSTRAINT_TYPE='UNIQUE'` の行が一致したら制約起源）。
//     CREATE UNIQUE INDEX で直接作られた UNIQUE インデックスは `table_constraints`
//     に対応行を持たないため、本フィルタを通り抜けて保持される。
//   - `INDEX_NAME, SEQ_IN_INDEX` で構成カラムの順序を保持（要件 7.2）。
//   - `IsUnique` は `NON_UNIQUE = 0` を採用（要件 7.3）。
const sqlSelectMySQLIndexes = `
SELECT
    s.TABLE_NAME,
    s.INDEX_NAME,
    s.NON_UNIQUE,
    s.COLUMN_NAME,
    s.SEQ_IN_INDEX
FROM information_schema.statistics s
LEFT JOIN information_schema.table_constraints tc
  ON tc.TABLE_SCHEMA    = s.TABLE_SCHEMA
 AND tc.TABLE_NAME      = s.TABLE_NAME
 AND tc.CONSTRAINT_NAME = s.INDEX_NAME
 AND tc.CONSTRAINT_TYPE = 'UNIQUE'
WHERE s.TABLE_SCHEMA = ?
  AND s.INDEX_NAME  != 'PRIMARY'
  AND tc.CONSTRAINT_NAME IS NULL
ORDER BY s.TABLE_NAME, s.INDEX_NAME, s.SEQ_IN_INDEX
`

// mysqlColumnRow は selectMySQLColumns が SQL 結果を Go 側で扱うための一時行。
// 取得後に normalizeMySQLAutoIncrement を経て rawColumn に変換する。
type mysqlColumnRow struct {
	Table      string
	Name       string
	Position   int
	ColumnType string
	IsNullable string
	Default    string
	Extra      string
	Comment    string
}

// mysqlFKRow は selectMySQLForeignKeys の 1 行を表す。同一 constraintName の
// カラムを position 昇順で集めて 1 件の rawForeignKey に組み立てる。
type mysqlFKRow struct {
	SourceTable    string
	ConstraintName string
	SourceColumn   string
	Position       int
	TargetTable    string
}

// mysqlIndexRow は selectMySQLIndexes の 1 行を表す。同一 (table, index) の
// カラムを position 昇順で集めて 1 件の rawIndex に組み立てる。
type mysqlIndexRow struct {
	Table     string
	IndexName string
	NonUnique int
	Column    string
	Position  int
}

// mysqlUniqueRow は selectMySQLSingleColumnUniques の 1 行を表す。
// 同一 (table, constraint) のカラム数が 1 のときだけ単一カラム UNIQUE と判定する。
type mysqlUniqueRow struct {
	Table          string
	ConstraintName string
	Column         string
}

// resolveMySQLSchema は対象スキーマ名を確定する。
//
// `schema` が空のときは `SELECT DATABASE()` で接続先 DB 名を解決する（要件 3.3）。
// 接続時に DB 未指定だった場合は NULL が返るため、説明的エラーを返す。
func resolveMySQLSchema(ctx context.Context, tx *sql.Tx, schema string) (string, error) {
	if schema != "" {
		return schema, nil
	}
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, sqlSelectMySQLDatabase).Scan(&current); err != nil {
		return "", fmt.Errorf("introspect/mysql: select database: %w", err)
	}
	if !current.Valid || current.String == "" {
		return "", fmt.Errorf("introspect/mysql: no database selected; specify --schema or include database in DSN")
	}
	return current.String, nil
}

// selectMySQLTables は対象スキーマに属する実テーブル名と TABLE_NAME -> TABLE_COMMENT
// の map を返す（要件 3.x / 8.2）。テーブル名は `information_schema.tables` の
// 規定順を保持する。
func selectMySQLTables(ctx context.Context, tx *sql.Tx, schema string) ([]string, map[string]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectMySQLTables, schema)
	if err != nil {
		return nil, nil, fmt.Errorf("introspect/mysql: select tables: %w", err)
	}
	defer rows.Close()
	names := make([]string, 0, 32)
	comments := map[string]string{}
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			return nil, nil, fmt.Errorf("introspect/mysql: select tables: scan: %w", err)
		}
		names = append(names, name)
		comments[name] = comment
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("introspect/mysql: select tables: rows: %w", err)
	}
	return names, comments, nil
}

// selectMySQLColumns はテーブル名キーで宣言順カラム列、および (table, column)
// キーでカラムコメントを返す（要件 4.x / 8.2）。
//
// AUTO_INCREMENT 列のデフォルト値抑止は normalizeMySQLAutoIncrement で適用する。
// 単一カラム UNIQUE 性は別段で `applySingleColumnUnique`／単一カラム UNIQUE
// 制約のマージで補完される（取得段階では UNIQUE 性は未確定）。
func selectMySQLColumns(ctx context.Context, tx *sql.Tx, schema string) (map[string][]rawColumn, map[tableColumnKey]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectMySQLColumns, schema)
	if err != nil {
		return nil, nil, fmt.Errorf("introspect/mysql: select columns: %w", err)
	}
	defer rows.Close()
	out := map[string][]rawColumn{}
	comments := map[tableColumnKey]string{}
	for rows.Next() {
		var r mysqlColumnRow
		if err := rows.Scan(&r.Table, &r.Name, &r.Position, &r.ColumnType, &r.IsNullable, &r.Default, &r.Extra, &r.Comment); err != nil {
			return nil, nil, fmt.Errorf("introspect/mysql: select columns: scan: %w", err)
		}
		defOut := normalizeMySQLAutoIncrement(r.Extra, r.Default)
		out[r.Table] = append(out[r.Table], rawColumn{
			Name:    r.Name,
			Type:    r.ColumnType,
			NotNull: r.IsNullable == "NO",
			Default: defOut,
		})
		comments[tableColumnKey{Table: r.Table, Column: r.Name}] = r.Comment
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("introspect/mysql: select columns: rows: %w", err)
	}
	return out, comments, nil
}

// selectMySQLPrimaryKeys はテーブル名キーで PK 構成カラム列を返す（要件 5.x）。
func selectMySQLPrimaryKeys(ctx context.Context, tx *sql.Tx, schema string) (map[string][]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectMySQLPrimaryKeys, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/mysql: select primary keys: %w", err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			return nil, fmt.Errorf("introspect/mysql: select primary keys: scan: %w", err)
		}
		out[tbl] = append(out[tbl], col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/mysql: select primary keys: rows: %w", err)
	}
	return out, nil
}

// selectMySQLSingleColumnUniques はテーブル名キーで「単一カラム UNIQUE 制約の
// 対象カラム名」配列を返す（要件 4.4）。複合 UNIQUE 制約はここでは除外する。
func selectMySQLSingleColumnUniques(ctx context.Context, tx *sql.Tx, schema string) (map[string][]string, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectMySQLUniqueConstraints, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/mysql: select unique constraints: %w", err)
	}
	defer rows.Close()
	type key struct {
		Table      string
		Constraint string
	}
	grouped := map[key][]string{}
	keysOrder := make([]key, 0, 16)
	for rows.Next() {
		var r mysqlUniqueRow
		if err := rows.Scan(&r.Table, &r.ConstraintName, &r.Column); err != nil {
			return nil, fmt.Errorf("introspect/mysql: select unique constraints: scan: %w", err)
		}
		k := key{Table: r.Table, Constraint: r.ConstraintName}
		if _, exists := grouped[k]; !exists {
			keysOrder = append(keysOrder, k)
		}
		grouped[k] = append(grouped[k], r.Column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/mysql: select unique constraints: rows: %w", err)
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

// selectMySQLForeignKeys はテーブル名キーで FK 列を返す（要件 6.x）。
//
// 同一 constraint_name のカラムを position 昇順で 1 件の rawForeignKey に集約する。
// 単一カラム FK の SourceUnique は `applyFKSourceUnique` で別段に補完するため、
// 取得段階では false とする。
func selectMySQLForeignKeys(ctx context.Context, tx *sql.Tx, schema string) (map[string][]rawForeignKey, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectMySQLForeignKeys, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/mysql: select foreign keys: %w", err)
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
		var r mysqlFKRow
		if err := rows.Scan(&r.SourceTable, &r.ConstraintName, &r.SourceColumn, &r.Position, &r.TargetTable); err != nil {
			return nil, fmt.Errorf("introspect/mysql: select foreign keys: scan: %w", err)
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
		return nil, fmt.Errorf("introspect/mysql: select foreign keys: rows: %w", err)
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

// selectMySQLIndexes はテーブル名キーで補助インデックス列を返す（要件 7.x）。
//
// 同一 index_name のカラムを SEQ_IN_INDEX 昇順で 1 件の rawIndex に集約する。
// IsUnique は `NON_UNIQUE = 0` を採用する（要件 7.3）。
func selectMySQLIndexes(ctx context.Context, tx *sql.Tx, schema string) (map[string][]rawIndex, error) {
	rows, err := tx.QueryContext(ctx, sqlSelectMySQLIndexes, schema)
	if err != nil {
		return nil, fmt.Errorf("introspect/mysql: select indexes: %w", err)
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
		var r mysqlIndexRow
		if err := rows.Scan(&r.Table, &r.IndexName, &r.NonUnique, &r.Column, &r.Position); err != nil {
			return nil, fmt.Errorf("introspect/mysql: select indexes: scan: %w", err)
		}
		k := key{Table: r.Table, Index: r.IndexName}
		e, exists := grouped[k]
		if !exists {
			e = &entry{IsUnique: r.NonUnique == 0}
			grouped[k] = e
			if _, seen := tableIndexes[r.Table]; !seen {
				tableOrder = append(tableOrder, r.Table)
			}
			tableIndexes[r.Table] = append(tableIndexes[r.Table], k)
		}
		e.Columns = append(e.Columns, r.Column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/mysql: select indexes: rows: %w", err)
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
