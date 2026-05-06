package introspect

import (
	"errors"
	"testing"
)

// TestDetectDriver は --driver 明示指定／DSN プレフィックス／ファイル拡張子の
// 各経路を表駆動で固定する（要件 1.5 / 2.4）。本格テストはタスク 9.1 で網羅するが、
// 本バッチでも契約固定の最小ケースを置き、後続バッチで破壊的変更が
// 起きないようにする（00-plan.md §"テスト方針"）。
func TestDetectDriver(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		dsn      string
		override string
		want     Driver
		wantErr  error
	}{
		{
			name:     "override postgres is honored",
			override: "postgres",
			dsn:      "anything",
			want:     DriverPostgreSQL,
		},
		{
			name:     "override mysql is honored",
			override: "mysql",
			dsn:      "anything",
			want:     DriverMySQL,
		},
		{
			name:     "override sqlite is honored",
			override: "sqlite",
			dsn:      "anything",
			want:     DriverSQLite,
		},
		{
			name:     "override unsupported value yields errUnsupportedDriver",
			override: "oracle",
			dsn:      "anything",
			wantErr:  errUnsupportedDriver,
		},
		{
			name: "postgres:// prefix infers PostgreSQL",
			dsn:  "postgres://user:pass@host:5432/db",
			want: DriverPostgreSQL,
		},
		{
			name: "postgresql:// prefix infers PostgreSQL",
			dsn:  "postgresql://user:pass@host:5432/db",
			want: DriverPostgreSQL,
		},
		{
			name: "mysql:// prefix infers MySQL",
			dsn:  "mysql://user:pass@host:3306/db",
			want: DriverMySQL,
		},
		{
			name: "file: prefix infers SQLite",
			dsn:  "file:./shop.sqlite?cache=shared",
			want: DriverSQLite,
		},
		{
			name: ".db extension infers SQLite",
			dsn:  "./var/data/shop.db",
			want: DriverSQLite,
		},
		{
			name: ".sqlite extension infers SQLite",
			dsn:  "shop.sqlite",
			want: DriverSQLite,
		},
		{
			name: ".sqlite3 extension infers SQLite",
			dsn:  "shop.sqlite3",
			want: DriverSQLite,
		},
		{
			name:    "unknown DSN yields errDriverInferenceFailed",
			dsn:     "user:pass@tcp(127.0.0.1:3306)/shop",
			wantErr: errDriverInferenceFailed,
		},
		// --- 以下、タスク 9.1 で追加した網羅ケース ---
		// 大文字小文字混在のプレフィックス（要件 1.5）。
		// `hasPostgresURLScheme` ／ `hasMySQLURLScheme` ／ `hasSQLiteFileURIPrefix`
		// はいずれも `strings.ToLower` で正規化してから比較するため、
		// ユーザーがどんな表記で DSN をコピペしても同じ判定になることを担保する。
		{
			name: "Postgres:// (mixed case) prefix infers PostgreSQL",
			dsn:  "Postgres://user:pass@host:5432/db",
			want: DriverPostgreSQL,
		},
		{
			name: "POSTGRESQL:// (uppercase) prefix infers PostgreSQL",
			dsn:  "POSTGRESQL://user:pass@host:5432/db",
			want: DriverPostgreSQL,
		},
		{
			name: "MYSQL:// (uppercase) prefix infers MySQL",
			dsn:  "MYSQL://user:pass@host:3306/db",
			want: DriverMySQL,
		},
		{
			name: "FILE: (uppercase) prefix infers SQLite",
			dsn:  "FILE:./shop.sqlite?cache=shared",
			want: DriverSQLite,
		},
		// 大文字小文字混在の拡張子（要件 1.5）。
		{
			name: ".DB (uppercase) extension infers SQLite",
			dsn:  "/var/data/shop.DB",
			want: DriverSQLite,
		},
		{
			name: ".SQLite (mixed case) extension infers SQLite",
			dsn:  "shop.SQLite",
			want: DriverSQLite,
		},
		{
			name: ".SQLite3 (mixed case) extension infers SQLite",
			dsn:  "shop.SQLite3",
			want: DriverSQLite,
		},
		// クエリパラメータ付き DSN（要件 1.5）。
		{
			name: "postgres URL with query params infers PostgreSQL",
			dsn:  "postgres://user:pass@host:5432/db?sslmode=disable&application_name=erdm",
			want: DriverPostgreSQL,
		},
		{
			name: "sqlite plain path with query-like suffix infers SQLite via extension",
			dsn:  "/var/data/shop.db?mode=ro",
			want: DriverSQLite,
		},
		// 既知のサポート外ドライバ override（要件 2.4）。
		{
			name:     "override sqlserver yields errUnsupportedDriver",
			override: "sqlserver",
			dsn:      "any",
			wantErr:  errUnsupportedDriver,
		},
		{
			name:     "override mongodb yields errUnsupportedDriver",
			override: "mongodb",
			dsn:      "any",
			wantErr:  errUnsupportedDriver,
		},
		// 大文字 override は enum 文字列と一致しないため未対応扱い（要件 2.4）。
		// CLI ヘルプでも小文字で受理することを明文化しているため、ここでも
		// その契約を固定する。
		{
			name:     "override Postgres (capitalized) yields errUnsupportedDriver",
			override: "Postgres",
			dsn:      "any",
			wantErr:  errUnsupportedDriver,
		},
		// 推定不能ケース（要件 1.5）。空 DSN + 空 override は CLI 側で先に
		// usage 出力して非ゼロ終了するが、本層でも防御的にエラーを返す。
		{
			name:    "empty DSN with empty override yields errDriverInferenceFailed",
			dsn:     "",
			wantErr: errDriverInferenceFailed,
		},
		{
			name:    "extensionless filename yields errDriverInferenceFailed",
			dsn:     "/var/data/database",
			wantErr: errDriverInferenceFailed,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := detectDriver(tc.dsn, tc.override)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("driver = %q, want %q", got, tc.want)
			}
		})
	}
}
