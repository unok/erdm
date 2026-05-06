package introspect

import (
	"path/filepath"
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
//   - SQLite: パスワード概念は持たないが、ファイルシステムレイアウトの
//     露出を避けるためディレクトリ部を落とし、ファイル名のみ（`file:` URI の
//     場合は `file:<basename>`）を返す（PR #24 レビュー指摘 / 要件 10.4）。
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
		return maskSQLiteDSN(dsn)
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

// maskSQLiteDSN は SQLite の DSN をエラーメッセージへ埋め込む際、ファイル
// システムレイアウト（ディレクトリ構成）が露出しないようファイル名のみを
// 残した安全表現を返す（要件 10.4 / PR #24 レビュー指摘）。
//
// 振る舞い:
//   - 空 DSN → 空文字列（呼び出し側で usage 分岐させるため）。
//   - パース不能な `file:` URI → 空文字列（原 DSN を漏らさないため）。
//   - `file:` URI（`file:./var/shop.db?cache=shared` など）→ `file:<basename>`
//     にクエリ・ディレクトリを落とした表現を返す。
//   - プレーンファイルパス（`./var/shop.db` 等）→ `<basename>` のみを返す。
//   - basename 抽出が `.`・`/`・空文字列に縮退した場合は空文字列で安全側に倒す。
//
// `:memory:` のような擬似 DSN は basename も `:memory:` のままなので、診断性を
// 損なわず情報を漏らさない。
func maskSQLiteDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	path, err := parseSQLiteDSN(dsn)
	if err != nil || path == "" {
		return ""
	}
	base := filepath.Base(path)
	if base == "" || base == "." || base == "/" {
		return ""
	}
	if hasSQLiteFileURIPrefix(dsn) {
		return "file:" + base
	}
	return base
}
