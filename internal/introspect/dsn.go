package introspect

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// dsn.go は PostgreSQL／MySQL／SQLite の DSN を分解する共有ヘルパを集約する。
//
// 共有の理由（design.md §"コンポーネントとインターフェース" / ナレッジ
// 「操作の一覧性」）:
//   - DSN 解析は `maskDSN`（パスワードマスク）と `resolveTitle`（タイトル
//     既定値）の両方で必要となる。各関数で別々にパースすると、後段の
//     ドライバ別 introspector（タスク 4.x ／ 5.x ／ 6.x）が増えるたびに
//     パース処理が散在する危険がある（REJECT 基準「同じ汎用関数が
//     目的の異なる 3 箇所以上から直接呼ばれている」）。
//   - 1 ファイル package private で集約することで、後続バッチも本ヘルパを
//     再利用しつつ「DSN を 2 度パースする」重複を避けられる。
//
// `mysql://` URL スキーム DSN の取り扱い（タスク 3.2）:
//   - 推定段階（`detectDriver`）では `mysql://` プレフィックスを MySQL として
//     識別する（要件 1.5）。
//   - しかし接続段階で受理する MySQL DSN は go-sql-driver/mysql の標準形式
//     `user:password@protocol(host:port)/dbname?param=value` のみであり、
//     `mysql://` URL からの正規化を試みると host／port／query／TLS 等の翻訳
//     誤りで利用者が想定しない DB へ接続する事故を生みうる。
//   - そこで本パッケージは「明示エラー方針」を採用する。`parseMySQLDSN` が
//     `mysql://` プレフィックスを検出した時点で `errMySQLURLSchemeNotSupported`
//     を返し、利用者には標準形式への置換を促すメッセージを提示する
//     （要件 2.2 ／ 11.2）。

// errMySQLURLSchemeNotSupported は `mysql://` URL スキーム DSN を接続段階で
// 拒否する際の sentinel エラー。`errors.Is(err, errMySQLURLSchemeNotSupported)`
// で呼び出し側が分岐できるよう提供する（タスク 3.2）。
var errMySQLURLSchemeNotSupported = errors.New("introspect: mysql:// URL scheme is not supported as a connection DSN; use 'user:password@tcp(host:port)/dbname' format instead")

// hasMySQLURLScheme は DSN が `mysql://` プレフィックスで始まるかを判定する。
// 大文字小文字を区別しないことで `MYSQL://...` のような表記も同一の境界に流す。
func hasMySQLURLScheme(dsn string) bool {
	return strings.HasPrefix(strings.ToLower(dsn), "mysql://")
}

// hasPostgresURLScheme は DSN が `postgres://` または `postgresql://` プレフィックス
// で始まるかを判定する。判定は大文字小文字非依存。
func hasPostgresURLScheme(dsn string) bool {
	lower := strings.ToLower(dsn)
	return strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://")
}

// hasSQLiteFileURIPrefix は DSN が SQLite の `file:` URI プレフィックスで始まるかを
// 判定する。modernc.org/sqlite は `file:foo.db?cache=shared` のような URI 表記を
// 受理するため、推定段階で SQLite として識別する（要件 1.5）。
func hasSQLiteFileURIPrefix(dsn string) bool {
	return strings.HasPrefix(strings.ToLower(dsn), "file:")
}

// parsePostgresDSN は PostgreSQL の URL 形式 DSN（`postgres://...` ／
// `postgresql://...`）を `*url.URL` へ分解する。本ヘルパは `maskDSN` と
// `resolveTitle` の両方から呼ばれ、DSN を 2 度パースする重複を避ける。
//
// 戻り値はパース失敗時に nil + error。エラーメッセージには原 DSN を含めず、
// 機密情報の漏洩を避ける（要件 10.4）。
func parsePostgresDSN(dsn string) (*url.URL, error) {
	if !hasPostgresURLScheme(dsn) {
		return nil, fmt.Errorf("introspect: not a postgres URL DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("introspect: failed to parse postgres DSN: %w", err)
	}
	return u, nil
}

// parseMySQLDSN は MySQL の DSN を `*mysql.Config` へ分解する共有窓口。
//
// 振る舞い:
//   - DSN が `mysql://` URL スキームで始まる場合、本ヘルパは
//     `errMySQLURLSchemeNotSupported` を返す（タスク 3.2 採用方針）。
//   - 標準形式（`user:password@protocol(host:port)/dbname[?params]`）の DSN は
//     `github.com/go-sql-driver/mysql` の `ParseDSN` でパースした結果を返す。
//
// 呼び出し側はエラー文言生成時に原 DSN を直接埋め込まず、`maskDSN(driver, dsn)`
// 経由のマスク済み文字列のみを使う規律を維持すること（要件 10.4）。
func parseMySQLDSN(dsn string) (*mysqldriver.Config, error) {
	if hasMySQLURLScheme(dsn) {
		return nil, errMySQLURLSchemeNotSupported
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("introspect: failed to parse mysql DSN: %w", err)
	}
	return cfg, nil
}

// parseSQLiteDSN は SQLite DSN（`file:` URI またはプレーンファイルパス）から
// 実体ファイルパスを抽出する。タイトル既定値解決（タスク 3.4）で
// `filepath.Base` の入力に使うことを想定する。
//
// 採用ルール:
//   - `file:` プレフィックスがあれば `net/url.Parse` で URL を分解し、
//     `Opaque` または `Path` のうち空でない側を採用する。Opaque は
//     `file:foo.db?cache=shared` のように authority を持たない相対パス DSN を
//     パースした結果に入る。Path は `file:///abs/foo.db` のような absolute
//     URI で得られる。
//   - プレーンパス（`./foo.db` 等）は素通しで返す。
func parseSQLiteDSN(dsn string) (string, error) {
	if !hasSQLiteFileURIPrefix(dsn) {
		return dsn, nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("introspect: failed to parse sqlite file URI: %w", err)
	}
	if u.Opaque != "" {
		return u.Opaque, nil
	}
	return u.Path, nil
}
