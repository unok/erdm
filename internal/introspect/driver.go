package introspect

import (
	"errors"
	"fmt"
	"strings"
)

// Driver はサポートする RDBMS 種別を表す値オブジェクト。
//
// 文字列値は CLI の --driver オプションで受理する小文字表記と一致させ、
// 利用者が CLI 入力からそのまま判定経路へ渡せる形にする（要件 1.5 / 2.4）。
type Driver string

// サポートするドライバ識別子。enum 風に const ブロックでまとめ、利用箇所を
// 1 箇所で見渡せるようにする（ナレッジ「操作の一覧性」）。
const (
	DriverUnknown    Driver = ""
	DriverPostgreSQL Driver = "postgres"
	DriverMySQL      Driver = "mysql"
	DriverSQLite     Driver = "sqlite"
)

// errUnsupportedDriver は --driver で受理できない値が渡された場合のエラー。
// 接続を試行する前段で検出して即座に返す（要件 2.4）。
var errUnsupportedDriver = errors.New("introspect: unsupported driver")

// errDriverInferenceFailed は DSN プレフィックス・ファイル拡張子のいずれからも
// ドライバを推定できなかった場合のエラー（要件 1.5）。
var errDriverInferenceFailed = errors.New("introspect: cannot infer driver from DSN")

// sqliteFileExtensions は SQLite と推定するファイル拡張子の一覧。
// 比較は小文字化したうえで行う（要件 1.5）。
var sqliteFileExtensions = []string{".db", ".sqlite", ".sqlite3"}

// detectDriver は --driver の明示指定値（override）と DSN を受け取り、
// 採用すべき Driver を確定して返す。
//
// 採用ルール（design.md §"要件トレーサビリティ" 1.5 / 2.4）:
//  1. override が空でなく既知の Driver と一致する → そのまま採用
//  2. override が空でなく既知の Driver と一致しない → errUnsupportedDriver
//  3. override が空 → DSN の URL スキームまたはファイル拡張子から推定
//  4. 推定できない → errDriverInferenceFailed
//
// MySQL の URL 形式（mysql://...）は推定段階で MySQL として識別するが、
// 接続段階では `parseMySQLDSN` が `errMySQLURLSchemeNotSupported` を返して
// 利用者へ標準 DSN への置換を促す（タスク 3.2 採用方針）。
func detectDriver(dsn string, override string) (Driver, error) {
	if override != "" {
		return detectFromOverride(override)
	}
	if d := detectFromPrefix(dsn); d != DriverUnknown {
		return d, nil
	}
	if d := detectFromExtension(dsn); d != DriverUnknown {
		return d, nil
	}
	return DriverUnknown, fmt.Errorf("%w", errDriverInferenceFailed)
}

// detectFromOverride は --driver 明示指定値を Driver enum へ突き合わせる。
// 既知でない値は `errUnsupportedDriver` でラップして返す（要件 2.4）。
func detectFromOverride(override string) (Driver, error) {
	switch Driver(override) {
	case DriverPostgreSQL, DriverMySQL, DriverSQLite:
		return Driver(override), nil
	default:
		return DriverUnknown, fmt.Errorf("%w: %q", errUnsupportedDriver, override)
	}
}

// detectFromPrefix は DSN の URL スキームから Driver を推定する。
// 該当しない場合は DriverUnknown を返す（要件 1.5）。
func detectFromPrefix(dsn string) Driver {
	switch {
	case hasPostgresURLScheme(dsn):
		return DriverPostgreSQL
	case hasMySQLURLScheme(dsn):
		return DriverMySQL
	case hasSQLiteFileURIPrefix(dsn):
		return DriverSQLite
	default:
		return DriverUnknown
	}
}

// detectFromExtension は DSN をファイルパスとみなし、末尾の拡張子から
// SQLite を推定する。`?` 以降のクエリ文字列は除去してから判定する。
func detectFromExtension(dsn string) Driver {
	path := dsn
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	lower := strings.ToLower(path)
	for _, ext := range sqliteFileExtensions {
		if strings.HasSuffix(lower, ext) {
			return DriverSQLite
		}
	}
	return DriverUnknown
}
