package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// sqlite_queries.go は SQLite 用の SELECT／PRAGMA 文と各クエリ実行ヘルパを集約する。
//
// 集約方針（ナレッジ「操作の一覧性」）:
//   - 関数内文字列リテラルに散らさず、関数外 const 定数として 1 ファイルに集約する。
//     SQL／PRAGMA 改変・レビュー時に SQLite が発行するクエリ全量を 1 箇所で見渡せる。
//   - PRAGMA は `SELECT * FROM pragma_xxx(?)` 形式で発行し、`database/sql` の標準
//     `Query` インターフェースで結果セットを取得する（`PRAGMA xxx(name)` 構文は
//     ドライバ依存が強いため避ける）。
//   - 各クエリ実行ヘルパは `*sql.DB` を受け取り、`sqliteIntrospector.fetch` の
//     オーケストレータが扱いやすい形に整形して返す。テーブル単位ループで
//     PRAGMA を発行する都合上、`(ctx, db, table)` のシグネチャを統一する。
//   - 失敗時は `fmt.Errorf("introspect/sqlite: <段階>: %w", err)` の規約で
//     ラップする（呼び出し側が段階を一目で識別できるようにする）。

// sqlSelectSQLiteTables は対象 DB の実テーブル名と CREATE TABLE 原文を取得する。
//
//   - `type = 'table'` でビューを除外（要件 3.2）。
//   - `name NOT LIKE 'sqlite_%'` でシステムテーブル（`sqlite_sequence` 等）と
//     一時表（`sqlite_temp_master` など）を除外する（要件 3.1）。
//   - SQL 原文（`sql` 列）はカラム宣言行末コメント抽出（要件 8.3）に渡すため
//     COALESCE で NULL を空文字列に正規化する。
//   - 取得順序は `sqlite_master` の規定順を採用する（要件 3.6）。
const sqlSelectSQLiteTables = `
SELECT name, COALESCE(sql, '')
FROM sqlite_master
WHERE type = 'table'
  AND name NOT LIKE 'sqlite_%'
`

// sqlSelectSQLiteColumns はテーブルのカラム情報を宣言順で取得する PRAGMA SELECT。
//
//   - `pragma_table_info(?)` のテーブル関数を `SELECT *` 経由で利用する。
//   - `cid`（宣言順）／`name`／`type`／`"notnull"`／`dflt_value`／`pk` を取得。
//   - `dflt_value` の NULL は SQL 側で空文字列に正規化（要件 4.5）。
//   - `pk` は 0=非PK、1..N=PK 構成順。0 を超える値が PK 構成カラム（要件 5.x）。
//   - 単一カラム PK で `type` が空または INTEGER（大小文字無視）のときは
//     自動 ROWID 連番列とみなしてデフォルト値を抑止する（要件 4.7、Go 側の純粋
//     ヘルパ `normalizeSQLiteAutoIncrement` で処理）。
const sqlSelectSQLiteColumns = `
SELECT cid, name, type, "notnull", COALESCE(dflt_value, ''), pk
FROM pragma_table_info(?)
ORDER BY cid
`

// sqlSelectSQLiteForeignKeys は外部キーを取得する PRAGMA SELECT。
//
//   - `pragma_foreign_key_list(?)` のテーブル関数を `SELECT *` 経由で利用する。
//   - `id` で複合 FK をグルーピングし、`seq` で構成カラム順序を確定する
//     （要件 6.4）。
//   - 参照先テーブル物理名は `"table"` 列から取得する。参照先がスコープ外
//     スキーマでも、テーブル物理名のみを返し、判定は呼び出し側
//     （タスク 7.4 buildSchema）に委ねる（要件 6.5）。
const sqlSelectSQLiteForeignKeys = `
SELECT id, seq, "table", "from"
FROM pragma_foreign_key_list(?)
ORDER BY id, seq
`

// sqlSelectSQLiteIndexList はテーブルに紐づくインデックスの一覧と起源を返す
// PRAGMA SELECT。
//
//   - `origin` は 'pk'（PK 由来）／'u'（UNIQUE 制約由来）／'c'（CREATE INDEX 由来）。
//   - 補助インデックスは `origin = 'c'` のみ採用する（要件 7.1）。
//   - `origin = 'u'` で構成カラム数 1 のインデックスから単一カラム UNIQUE 制約を
//     導出する（要件 4.4）。
//   - `unique` は 0/1。`IsUnique` は `unique != 0` で採用する（要件 7.3）。
const sqlSelectSQLiteIndexList = `
SELECT name, "unique", origin
FROM pragma_index_list(?)
ORDER BY seq
`

// sqlSelectSQLiteIndexInfo はインデックスの構成カラムを `seqno` 順で返す
// PRAGMA SELECT。
//
//   - `name` 列は通常列インデックスでは物理カラム名、式インデックスでは NULL。
//   - 本ツールは式インデックスを構成カラムを持たないものとして扱い、`name` が
//     NULL の行は読み飛ばす（要件 7.2 の「構成カラム順序」が定義できないため）。
const sqlSelectSQLiteIndexInfo = `
SELECT seqno, cid, name
FROM pragma_index_info(?)
ORDER BY seqno
`

// sqliteColumnRow は selectSQLiteColumns が SQL 結果を Go 側で扱うための一時行。
type sqliteColumnRow struct {
	CID     int
	Name    string
	Type    string
	NotNull int
	Default string
	PK      int
}

// sqliteFKRow は selectSQLiteForeignKeys の 1 行を表す。同一 id のカラムを
// seq 昇順で集めて 1 件の rawForeignKey に組み立てる。
type sqliteFKRow struct {
	ID          int
	Seq         int
	TargetTable string
	From        string
}

// sqliteIndexListRow は selectSQLiteIndexes が pragma_index_list を読むための一時行。
type sqliteIndexListRow struct {
	Name     string
	IsUnique int
	Origin   string
}

// selectSQLiteTables は対象 DB の実テーブル名と CREATE TABLE 原文 map を返す
// （要件 3.x）。テーブル名は `sqlite_master` の規定順を保持する。
func selectSQLiteTables(ctx context.Context, db *sql.DB) ([]string, map[string]string, error) {
	rows, err := db.QueryContext(ctx, sqlSelectSQLiteTables)
	if err != nil {
		return nil, nil, fmt.Errorf("introspect/sqlite: select tables: %w", err)
	}
	defer rows.Close()
	names := make([]string, 0, 16)
	ddls := map[string]string{}
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			return nil, nil, fmt.Errorf("introspect/sqlite: select tables: scan: %w", err)
		}
		names = append(names, name)
		ddls[name] = ddl
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("introspect/sqlite: select tables: rows: %w", err)
	}
	return names, ddls, nil
}

// selectSQLiteColumns は対象テーブルのカラム列と PK 構成カラム列を返す
// （要件 4.x / 5.x）。
//
// 自動 ROWID 連番列（単一 PK で型が空または INTEGER）は
// `normalizeSQLiteAutoIncrement` でデフォルト値を抑止し、空型は INTEGER に
// 正規化する（要件 4.7）。単一カラム UNIQUE 性は別段の取得経路で
// `applySingleColumnUnique`／単一カラム UNIQUE 制約の合算で補完される。
func selectSQLiteColumns(ctx context.Context, db *sql.DB, table string) ([]rawColumn, []string, error) {
	rows, err := db.QueryContext(ctx, sqlSelectSQLiteColumns, table)
	if err != nil {
		return nil, nil, fmt.Errorf("introspect/sqlite: pragma table_info: %w", err)
	}
	defer rows.Close()
	var cols []rawColumn
	var pkSeqs []int
	for rows.Next() {
		var r sqliteColumnRow
		if err := rows.Scan(&r.CID, &r.Name, &r.Type, &r.NotNull, &r.Default, &r.PK); err != nil {
			return nil, nil, fmt.Errorf("introspect/sqlite: pragma table_info: scan: %w", err)
		}
		cols = append(cols, rawColumn{
			Name:    r.Name,
			Type:    r.Type,
			NotNull: r.NotNull != 0,
			Default: r.Default,
		})
		pkSeqs = append(pkSeqs, r.PK)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("introspect/sqlite: pragma table_info: rows: %w", err)
	}
	pk := derivePrimaryKey(cols, pkSeqs)
	applyAutoIncrementToColumns(cols, pkSeqs, len(pk) == 1)
	return cols, pk, nil
}

// derivePrimaryKey は pragma_table_info.pk 値から PK 構成カラム名を順序付きで返す
// 純粋ヘルパ。pk == 0 のカラムを除外し、pk 値で昇順整列する（要件 5.2）。
func derivePrimaryKey(cols []rawColumn, pkSeqs []int) []string {
	type pkPair struct {
		name string
		seq  int
	}
	pairs := make([]pkPair, 0, len(cols))
	for i, p := range pkSeqs {
		if p > 0 {
			pairs = append(pairs, pkPair{name: cols[i].Name, seq: p})
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].seq < pairs[j].seq })
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.name
	}
	return out
}

// applyAutoIncrementToColumns は単一カラム PK に対して
// normalizeSQLiteAutoIncrement を適用する純粋ヘルパ。複合 PK では適用しない
// （要件 4.7）。
func applyAutoIncrementToColumns(cols []rawColumn, pkSeqs []int, isSinglePK bool) {
	for i := range cols {
		typeOut, defOut := normalizeSQLiteAutoIncrement(cols[i].Type, cols[i].Default, pkSeqs[i] > 0, isSinglePK)
		cols[i].Type = typeOut
		cols[i].Default = defOut
	}
}

// selectSQLiteForeignKeys は対象テーブルの外部キー列を返す（要件 6.x）。
//
// 同一 id のカラムを seq 昇順で 1 件の rawForeignKey に集約する。複合 FK は
// SourceColumns の長さで表現される。`SourceUnique` は取得段階では false とし、
// `applyFKSourceUnique` が rawColumn.IsUnique 確定後に補完する。
func selectSQLiteForeignKeys(ctx context.Context, db *sql.DB, table string) ([]rawForeignKey, error) {
	rows, err := db.QueryContext(ctx, sqlSelectSQLiteForeignKeys, table)
	if err != nil {
		return nil, fmt.Errorf("introspect/sqlite: pragma foreign_key_list: %w", err)
	}
	defer rows.Close()
	type entry struct {
		TargetTable string
		Columns     []string
	}
	grouped := map[int]*entry{}
	order := make([]int, 0, 4)
	for rows.Next() {
		var r sqliteFKRow
		if err := rows.Scan(&r.ID, &r.Seq, &r.TargetTable, &r.From); err != nil {
			return nil, fmt.Errorf("introspect/sqlite: pragma foreign_key_list: scan: %w", err)
		}
		e, ok := grouped[r.ID]
		if !ok {
			e = &entry{TargetTable: r.TargetTable}
			grouped[r.ID] = e
			order = append(order, r.ID)
		}
		e.Columns = append(e.Columns, r.From)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/sqlite: pragma foreign_key_list: rows: %w", err)
	}
	out := make([]rawForeignKey, 0, len(order))
	for _, id := range order {
		e := grouped[id]
		out = append(out, rawForeignKey{
			SourceColumns: e.Columns,
			TargetTable:   e.TargetTable,
		})
	}
	return out, nil
}

// selectSQLiteIndexes は対象テーブルの補助インデックス列、および UNIQUE 制約由来
// の単一カラム名列を返す（要件 4.4 / 7.x）。
//
//   - origin='c': 補助インデックスとして rawIndex に追加する。
//   - origin='u': 構成カラムが 1 個のときに単一カラム UNIQUE 制約と判定し、
//     対応カラム名を返す。複数カラムのときは無視する（要件 4.4）。
//   - origin='pk': PK 由来のインデックスは除外（要件 7.1）。
func selectSQLiteIndexes(ctx context.Context, db *sql.DB, table string) ([]rawIndex, []string, error) {
	metas, err := selectSQLiteIndexList(ctx, db, table)
	if err != nil {
		return nil, nil, err
	}
	var indexes []rawIndex
	var uniqueCols []string
	for _, m := range metas {
		cols, err := selectSQLiteIndexInfo(ctx, db, m.Name)
		if err != nil {
			return nil, nil, err
		}
		switch m.Origin {
		case "c":
			if len(cols) == 0 {
				continue
			}
			indexes = append(indexes, rawIndex{
				Name:     m.Name,
				Columns:  cols,
				IsUnique: m.IsUnique != 0,
			})
		case "u":
			if len(cols) == 1 {
				uniqueCols = append(uniqueCols, cols[0])
			}
		}
	}
	return indexes, uniqueCols, nil
}

// selectSQLiteIndexList は pragma_index_list の結果を行型のスライスで返す。
// 行は ResultSet を閉じてから後続のクエリ（pragma_index_info）を発行できるよう、
// 一旦メモリに格納する。
func selectSQLiteIndexList(ctx context.Context, db *sql.DB, table string) ([]sqliteIndexListRow, error) {
	rows, err := db.QueryContext(ctx, sqlSelectSQLiteIndexList, table)
	if err != nil {
		return nil, fmt.Errorf("introspect/sqlite: pragma index_list: %w", err)
	}
	defer rows.Close()
	var metas []sqliteIndexListRow
	for rows.Next() {
		var r sqliteIndexListRow
		if err := rows.Scan(&r.Name, &r.IsUnique, &r.Origin); err != nil {
			return nil, fmt.Errorf("introspect/sqlite: pragma index_list: scan: %w", err)
		}
		metas = append(metas, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/sqlite: pragma index_list: rows: %w", err)
	}
	return metas, nil
}

// selectSQLiteIndexInfo は対象インデックスの構成カラムを seqno 順で返す。
// 式インデックスのカラム（name が NULL）は構成カラム順序が定義できないため
// 読み飛ばす（要件 7.2）。
func selectSQLiteIndexInfo(ctx context.Context, db *sql.DB, indexName string) ([]string, error) {
	rows, err := db.QueryContext(ctx, sqlSelectSQLiteIndexInfo, indexName)
	if err != nil {
		return nil, fmt.Errorf("introspect/sqlite: pragma index_info: %w", err)
	}
	defer rows.Close()
	cols := make([]string, 0, 4)
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, fmt.Errorf("introspect/sqlite: pragma index_info: scan: %w", err)
		}
		if name.Valid {
			cols = append(cols, name.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("introspect/sqlite: pragma index_info: rows: %w", err)
	}
	return cols, nil
}
