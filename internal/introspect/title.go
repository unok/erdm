package introspect

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveTitle はスキーマタイトルの既定値を解決する（要件 9.5 / 9.6）。
//
// 採用順序（design.md §"要件トレーサビリティ" 9.5 / 9.6）:
//  1. opts.Title が空でない → そのまま採用
//  2. PostgreSQL / MySQL → 同 DSN パース基盤を maskDSN と共有しつつ
//     接続先 DB 名を抽出して採用
//  3. SQLite → ファイルパスのベース部（拡張子除去後）を採用
//  4. いずれも不能 → 空文字列を返し、呼び出し側で usage / エラー扱い
//
// maskDSN と DSN パース基盤を共有することで「DSN を 2 度パースする」
// 重複を避ける（ナレッジ「解決責務の一元化」）。
func resolveTitle(driver Driver, opts Options) (string, error) {
	if opts.Title != "" {
		return opts.Title, nil
	}
	switch driver {
	case DriverPostgreSQL:
		return titleFromPostgresDSN(opts.DSN)
	case DriverMySQL:
		return titleFromMySQLDSN(opts.DSN)
	case DriverSQLite:
		return titleFromSQLiteDSN(opts.DSN)
	default:
		return "", fmt.Errorf("introspect: cannot resolve title for driver %q", driver)
	}
}

// titleFromPostgresDSN は `postgres://.../<dbname>?...` のパスから接続先 DB 名を
// 抽出する。パスが空の場合は空文字列を返し、呼び出し側で usage を促す。
func titleFromPostgresDSN(dsn string) (string, error) {
	u, err := parsePostgresDSN(dsn)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

// titleFromMySQLDSN は MySQL の標準形式 DSN から DBName を抽出する。
// `mysql://` URL は `parseMySQLDSN` が `errMySQLURLSchemeNotSupported` を
// 返すため、本関数は同 sentinel をそのまま伝播する（タスク 3.2）。
func titleFromMySQLDSN(dsn string) (string, error) {
	cfg, err := parseMySQLDSN(dsn)
	if err != nil {
		return "", err
	}
	return cfg.DBName, nil
}

// titleFromSQLiteDSN は SQLite DSN（`file:` URI またはプレーンパス）から
// ファイル名のベース部を取り出し、`.db` ／ `.sqlite` ／ `.sqlite3` の
// 拡張子を除去した文字列を返す。
func titleFromSQLiteDSN(dsn string) (string, error) {
	path, err := parseSQLiteDSN(dsn)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	for _, ext := range sqliteFileExtensions {
		if strings.HasSuffix(lower, ext) {
			return base[:len(base)-len(ext)], nil
		}
	}
	return base, nil
}
