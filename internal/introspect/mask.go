package introspect

import (
	"strings"
)

// maskedPasswordPlaceholder はマスク後のパスワード位置に埋め込む固定文字列。
// 利用者がエラーメッセージを見た際、ここがパスワードであったと識別できる
// 程度に目立つ表現を採用する（要件 2.5）。
const maskedPasswordPlaceholder = "***"

// maskDSN は接続失敗等のエラーメッセージに DSN を埋め込む際、原 DSN の
// パスワード部分を伏字（"***"）へ置換した安全な表現を返す（要件 2.5 / 10.4）。
//
// ドライバごとの DSN 形式に応じて以下の戦略を採る（design.md §"要件
// トレーサビリティ" 2.5）。
//   - PostgreSQL（postgres:// / postgresql://）: net/url で URL を分解し、
//     User.Password を *** に差し替えて再構築する。
//   - MySQL（user:pass@tcp(host:port)/db）: github.com/go-sql-driver/mysql の
//     mysql.Config 経由でパース → Passwd を伏字に置換 → FormatDSN で再構築。
//     mysql:// URL 表記は `parseMySQLDSN` が `errMySQLURLSchemeNotSupported`
//     を返すので、本関数では空文字列を返し、呼び出し側でエラー文言を
//     生成させる（タスク 3.2 採用方針）。
//   - SQLite: パスワード概念を持たないファイルパス／file: URL を返す。
//
// 戻り値はエラーメッセージにそのまま埋め込める安全な文字列。原 DSN を
// 露出させないためのバリアであり、呼び出し側は原 DSN を直接ログ／エラーへ
// 出さないこと（ナレッジ「Fail Fast / 機密データの取り扱い」）。
func maskDSN(driver Driver, dsn string) string {
	switch driver {
	case DriverPostgreSQL:
		return maskPostgresDSN(dsn)
	case DriverMySQL:
		return maskMySQLDSN(dsn)
	case DriverSQLite:
		return dsn
	default:
		// 未指定ドライバへ原 DSN をそのまま漏らさない。空文字列で安全側に倒す。
		return ""
	}
}

// maskPostgresDSN は PostgreSQL の URL 形式 DSN のパスワード部を伏字へ
// 置換した文字列を返す。パース失敗時は原 DSN 漏洩を避けるため空文字列を返す。
//
// `url.UserPassword` 経由の置換だとアスタリスクが sub-delims として
// パーセントエンコードされ `%2A%2A%2A` となってしまうため、Userinfo を
// nil クリアした URL を文字列化したうえで `://` 直後に「user:***@」を
// 手挿入する方式を採用する。
func maskPostgresDSN(dsn string) string {
	u, err := parsePostgresDSN(dsn)
	if err != nil {
		return ""
	}
	if u.User == nil {
		return u.String()
	}
	username := u.User.Username()
	if _, hasPassword := u.User.Password(); !hasPassword {
		return u.String()
	}
	u.User = nil
	withoutUser := u.String()
	const sep = "://"
	idx := strings.Index(withoutUser, sep)
	if idx < 0 {
		return ""
	}
	return withoutUser[:idx+len(sep)] + username + ":" + maskedPasswordPlaceholder + "@" + withoutUser[idx+len(sep):]
}

// maskMySQLDSN は MySQL の標準 DSN（`user:password@protocol(host:port)/db`）の
// パスワード位置を伏字に置換する。`mysql://` URL は本ヘルパでは扱わない
// （`parseMySQLDSN` が `errMySQLURLSchemeNotSupported` を返す）。
func maskMySQLDSN(dsn string) string {
	cfg, err := parseMySQLDSN(dsn)
	if err != nil {
		return ""
	}
	if cfg.Passwd == "" {
		return cfg.FormatDSN()
	}
	cfg.Passwd = maskedPasswordPlaceholder
	return cfg.FormatDSN()
}
