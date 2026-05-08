// Package introspect は稼働中の RDBMS（PostgreSQL / MySQL / SQLite）に
// 接続し、テーブル・カラム・主キー・外部キー・インデックス・コメントを取得して
// 既存の *model.Schema を構築するための独立パッケージ。
//
// 設計の主旨（design.md §"アーキテクチャ" / 要件 12.x）:
//   - DB ドライバ依存はこのパッケージと erdm.go の blank import の 2 箇所のみに
//     閉じ込め、internal/{parser,serializer,model,server,...} 等を汚染しない
//     （要件 12.1）。
//   - 公開 API はドメイン操作 1 関数 Introspect のみ。値オブジェクト Options と
//     Driver enum はシグネチャ上必要となるため公開する（要件 12.2）。
//   - ドライバ別の SELECT/PRAGMA 実装は postgres.go / mysql.go / sqlite.go の
//     ファイル単位で分離し、共通の DTO（types.go）と builder（builder.go）に
//     集約変換する（ナレッジ「1 ファイルに 1 責務」）。
//
// 採用ドライバ（research.md / design.md §"技術スタック"）:
//   - PostgreSQL: github.com/jackc/pgx/v5/stdlib（CGO 不要・active-maintained・
//     `postgres://` URL DSN 受理）
//   - MySQL: github.com/go-sql-driver/mysql（事実上の標準・CGO 不要・
//     `user:pass@tcp(host:port)/db` 形式の標準 DSN 受理）
//   - SQLite: modernc.org/sqlite（純 Go 実装・CGO 不要・gox クロスコンパイル維持）
package introspect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/unok/erdm/internal/model"
)

// driverNames は Driver と database/sql 登録名の対応表。
//
// `pgx` は `github.com/jackc/pgx/v5/stdlib` の登録名、`mysql` は
// `github.com/go-sql-driver/mysql` の登録名、`sqlite` は `modernc.org/sqlite`
// の登録名。blank import は erdm.go に集約してあり、本パッケージから
// 直接 import しないことで「DB ドライバ依存は erdm.go と本パッケージ
// の 2 箇所のみ」（要件 12.3）を満たす。
var driverNames = map[Driver]string{
	DriverPostgreSQL: "pgx",
	DriverMySQL:      "mysql",
	DriverSQLite:     "sqlite",
}

// Introspect は Options で指定された DSN 接続先 RDBMS のスキーマを取得し、
// 既存パーサ／シリアライザと整合する *model.Schema を返す唯一の公開操作。
//
// パイプラインは「ドライバ確定 → タイトル解決 → DB 接続 → ドライバ別
// イントロスペクタ実行 → 内部 DTO → builder によるドメインモデル変換」の
// 直線的フロー。失敗時は maskDSN を経由したエラーを返し、原 DSN を
// 露出させない（要件 10.4）。
func Introspect(ctx context.Context, opts Options) (*model.Schema, error) {
	if opts.DSN == "" {
		return nil, errors.New("introspect: dsn is required")
	}
	driver, err := detectDriver(opts.DSN, string(opts.Driver))
	if err != nil {
		return nil, fmt.Errorf("introspect: %w", err)
	}
	opts.Driver = driver

	title, err := resolveTitle(driver, opts)
	if err != nil {
		return nil, fmt.Errorf("introspect: resolve title: %w", err)
	}
	opts.Title = title

	db, err := openDB(driver, opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("introspect: open %s: %w", maskDSN(driver, opts.DSN), err)
	}
	defer func() { _ = db.Close() }()

	raws, err := fetchRawTables(ctx, driver, db, opts.Schema)
	if err != nil {
		return nil, fmt.Errorf("introspect: fetch %s: %w", maskDSN(driver, opts.DSN), err)
	}

	known := buildKnownTables(raws)
	schema, err := buildSchema(raws, opts, known)
	if err != nil {
		return nil, err
	}
	if !opts.NoInferFK {
		inferNamingConventionFKs(schema)
	}
	return schema, nil
}

// openDB は driverNames を介して `database/sql.Open` を呼び出す共有ヘルパ。
// 未登録ドライバが指定された場合は errUnsupportedDriver を返す（防御的実装）。
func openDB(driver Driver, dsn string) (*sql.DB, error) {
	name, ok := driverNames[driver]
	if !ok {
		return nil, fmt.Errorf("%w: %q", errUnsupportedDriver, driver)
	}
	return sql.Open(name, dsn)
}

// fetchRawTables は driver に対応するイントロスペクタを生成し、rawTable 列を取得する。
//
// SQLite はスキーマ概念を持たないため schema 引数は無視する。PostgreSQL ／
// MySQL は schema が空文字のときドライバごとの既定（PG: public、MySQL:
// 接続先 DB）を採用する（要件 3.3 / 3.4）。
func fetchRawTables(ctx context.Context, driver Driver, db *sql.DB, schema string) ([]rawTable, error) {
	switch driver {
	case DriverPostgreSQL:
		return newPostgresIntrospector(db, schema).fetch(ctx)
	case DriverMySQL:
		return newMySQLIntrospector(db, schema).fetch(ctx)
	case DriverSQLite:
		return newSQLiteIntrospector(db).fetch(ctx)
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedDriver, driver)
	}
}

// buildKnownTables は rawTable 列から物理名集合を構築する。
// `buildSchema` の knownTables 引数（スコープ外参照 FK の判定）に渡す。
func buildKnownTables(raws []rawTable) map[string]struct{} {
	known := make(map[string]struct{}, len(raws))
	for _, r := range raws {
		known[r.Name] = struct{}{}
	}
	return known
}
