package introspect

import "testing"

// TestMaskDSN はドライバ別の DSN パスワードマスク契約を固定する
// （要件 2.5 / 10.4）。本格テストはタスク 9.2 で網羅するが、本バッチでも
// 主要ケースを表駆動で固定し後続バッチでの破壊を防ぐ（00-plan.md
// §"テスト方針"）。
func TestMaskDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		driver Driver
		dsn    string
		want   string
	}{
		{
			name:   "postgres URL with password is masked",
			driver: DriverPostgreSQL,
			dsn:    "postgres://user:secret@host:5432/db?sslmode=disable",
			want:   "postgres://user:***@host:5432/db?sslmode=disable",
		},
		{
			name:   "postgres URL without password is returned as-is",
			driver: DriverPostgreSQL,
			dsn:    "postgres://user@host:5432/db",
			want:   "postgres://user@host:5432/db",
		},
		{
			name:   "postgres URL without user is returned as-is",
			driver: DriverPostgreSQL,
			dsn:    "postgres://host:5432/db",
			want:   "postgres://host:5432/db",
		},
		{
			name:   "mysql standard DSN with password is masked",
			driver: DriverMySQL,
			dsn:    "user:secret@tcp(127.0.0.1:3306)/shop",
			want:   "user:***@tcp(127.0.0.1:3306)/shop",
		},
		{
			name:   "mysql:// URL DSN yields empty string (caller emits error)",
			driver: DriverMySQL,
			dsn:    "mysql://user:secret@host:3306/db",
			want:   "",
		},
		// SQLite はディレクトリ構成を露出させないため、basename のみを残す
		// （PR #24 レビュー指摘 / 要件 10.4）。
		{
			name:   "sqlite file path is masked to basename",
			driver: DriverSQLite,
			dsn:    "./var/shop.db",
			want:   "shop.db",
		},
		{
			name:   "sqlite absolute path is masked to basename",
			driver: DriverSQLite,
			dsn:    "/srv/data/inventory.sqlite3",
			want:   "inventory.sqlite3",
		},
		{
			name:   "sqlite file URI drops directory and query",
			driver: DriverSQLite,
			dsn:    "file:./var/shop.db?cache=shared",
			want:   "file:shop.db",
		},
		{
			name:   "sqlite absolute file URI is masked to file:<basename>",
			driver: DriverSQLite,
			dsn:    "file:///abs/path/shop.db",
			want:   "file:shop.db",
		},
		{
			name:   "sqlite :memory: is preserved (no path leak)",
			driver: DriverSQLite,
			dsn:    ":memory:",
			want:   ":memory:",
		},
		{
			name:   "unknown driver yields empty string for safety",
			driver: DriverUnknown,
			dsn:    "postgres://user:secret@host/db",
			want:   "",
		},
		// --- 以下、タスク 9.2 で追加した網羅ケース ---
		// パスワードに記号を含むケース。URL DSN ではパスワード中の特殊文字は
		// パーセントエンコードされるため、原 DSN が `%40` を含んでも本関数は
		// パスワード本体を伏字化したうえで username を保持し、ホスト以降は
		// `url.URL.String()` の再構築結果を採用する。
		// （要件 2.5：原 DSN を漏らさず、伏字化された安全な表現を返す）。
		{
			name:   "postgres URL password with URL-encoded @ is masked",
			driver: DriverPostgreSQL,
			dsn:    "postgres://user:p%40ssword@host:5432/db",
			want:   "postgres://user:***@host:5432/db",
		},
		{
			name:   "postgres URL password with URL-encoded slash is masked",
			driver: DriverPostgreSQL,
			dsn:    "postgres://user:p%2Fass@host:5432/db",
			want:   "postgres://user:***@host:5432/db",
		},
		{
			name:   "postgres URL password with URL-encoded colon is masked",
			driver: DriverPostgreSQL,
			dsn:    "postgres://user:p%3Aass@host:5432/db",
			want:   "postgres://user:***@host:5432/db",
		},
		// パース不能な PG URL は原 DSN を漏らさず空文字列で安全側に倒す
		// （要件 10.4）。`url.Parse` が失敗するのは主にホスト部の不整合など
		// 限定的なケースだが、空文字列フォールバックを契約として固定する。
		{
			name:   "unparseable postgres DSN yields empty string",
			driver: DriverPostgreSQL,
			dsn:    "postgres://[invalid-host",
			want:   "",
		},
		// 大文字 `MYSQL://` も `parseMySQLDSN` 経由で
		// `errMySQLURLSchemeNotSupported` となり、本関数は空文字列を返す。
		{
			name:   "MYSQL:// (uppercase) URL DSN yields empty string",
			driver: DriverMySQL,
			dsn:    "MYSQL://user:secret@host:3306/db",
			want:   "",
		},
		// MySQL 標準 DSN でパスワード未設定（`:` 区切りなし）。
		// go-sql-driver/mysql は `user@tcp(...)/db` を Passwd="" でパースし、
		// FormatDSN は同形式を返す。原 DSN そのままで安全（要件 2.5）。
		{
			name:   "mysql standard DSN without password is returned as-is",
			driver: DriverMySQL,
			dsn:    "user@tcp(127.0.0.1:3306)/shop",
			want:   "user@tcp(127.0.0.1:3306)/shop",
		},
		// 不正な MySQL 標準 DSN は `parseMySQLDSN` がエラーを返すため空文字列。
		// `ParseDSN` は形式逸脱（`tcp(` 開いて `)` 閉じない等）でエラーになる。
		{
			name:   "malformed mysql DSN yields empty string",
			driver: DriverMySQL,
			dsn:    "this-is-not-a-mysql-dsn",
			want:   "",
		},
		// SQLite 空文字列も安全に素通し（要件は明示しないが、原 DSN 漏洩
		// 防止と Fail Fast の観点で「空 → 空」が自然）。
		{
			name:   "sqlite empty string is returned as-is",
			driver: DriverSQLite,
			dsn:    "",
			want:   "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maskDSN(tc.driver, tc.dsn)
			if got != tc.want {
				t.Fatalf("maskDSN(%q, %q) = %q, want %q", tc.driver, tc.dsn, got, tc.want)
			}
		})
	}
}
