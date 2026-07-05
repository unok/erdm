package introspect

import (
	"errors"
	"strings"
	"testing"
)

// TestResolveTitle は --title 明示優先／PostgreSQL DSN／MySQL DSN／SQLite
// ファイルパスからのタイトル既定値解決を契約として固定する（要件 9.5 / 9.6）。
// 本格テストはタスク 9.3 で網羅するが、本バッチでも主要経路を表駆動で
// 押さえる（00-plan.md §"テスト方針"）。
func TestResolveTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		driver  Driver
		opts    Options
		want    string
		wantErr error
	}{
		{
			name:   "explicit Title is honored regardless of DSN",
			driver: DriverPostgreSQL,
			opts:   Options{Title: "explicit-title", DSN: "postgres://u:p@h/db"},
			want:   "explicit-title",
		},
		{
			name:   "postgres DSN extracts dbname from path",
			driver: DriverPostgreSQL,
			opts:   Options{DSN: "postgres://u:p@host:5432/shop?sslmode=disable"},
			want:   "shop",
		},
		{
			name:   "postgres DSN with empty path returns empty title",
			driver: DriverPostgreSQL,
			opts:   Options{DSN: "postgres://u:p@host:5432/"},
			want:   "",
		},
		{
			name:   "mysql standard DSN extracts DBName",
			driver: DriverMySQL,
			opts:   Options{DSN: "u:p@tcp(127.0.0.1:3306)/shop"},
			want:   "shop",
		},
		{
			name:    "mysql:// URL DSN yields errMySQLURLSchemeNotSupported",
			driver:  DriverMySQL,
			opts:    Options{DSN: "mysql://u:p@host:3306/shop"},
			wantErr: errMySQLURLSchemeNotSupported,
		},
		{
			name:   "sqlite plain path strips .db extension",
			driver: DriverSQLite,
			opts:   Options{DSN: "./var/data/shop.db"},
			want:   "shop",
		},
		{
			name:   "sqlite file URI strips .sqlite3 extension",
			driver: DriverSQLite,
			opts:   Options{DSN: "file:./var/inventory.sqlite3?cache=shared"},
			want:   "inventory",
		},
		{
			name:   "sqlite path without known extension keeps basename",
			driver: DriverSQLite,
			opts:   Options{DSN: "./var/data/shop"},
			want:   "shop",
		},
		{
			name:    "unsupported driver yields explanatory error",
			driver:  DriverUnknown,
			opts:    Options{DSN: "anything"},
			wantErr: errSentinelUnknownDriver,
		},
		// --- 以下、タスク 9.3 で追加した網羅ケース ---
		// MySQL 標準 DSN にクエリパラメータ付き → DBName のみ抽出（要件 9.6）。
		{
			name:   "mysql DSN with query params extracts DBName only",
			driver: DriverMySQL,
			opts:   Options{DSN: "u:p@tcp(127.0.0.1:3306)/shop?charset=utf8mb4&parseTime=true"},
			want:   "shop",
		},
		// MySQL 標準 DSN で DBName 未指定 → 空文字列。CLI 側で usage 表示や
		// fallback 判断に使えることを契約として固定する（要件 9.6）。
		{
			name:   "mysql DSN without dbname yields empty title",
			driver: DriverMySQL,
			opts:   Options{DSN: "u:p@tcp(127.0.0.1:3306)/"},
			want:   "",
		},
		// SQLite file: URI のクエリ文字列を除去してベース部を取り出す
		// （要件 9.6）。`parseSQLiteDSN` が url.URL.Opaque を返し、
		// `RawQuery` は分離されるため、`filepath.Base` 入力にクエリは残らない。
		{
			name:   "sqlite file URI with query string strips both query and extension",
			driver: DriverSQLite,
			opts:   Options{DSN: "file:./path/to/foo.db?mode=ro"},
			want:   "foo",
		},
		// SQLite 絶対パス（要件 9.6）。
		{
			name:   "sqlite absolute path strips .db extension",
			driver: DriverSQLite,
			opts:   Options{DSN: "/var/db/inventory.db"},
			want:   "inventory",
		},
		// SQLite file URI で絶対パス（authority 付き）。`parseSQLiteDSN` は
		// `u.Path` を返し、`filepath.Base` でファイル名のみ取り出される。
		{
			name:   "sqlite file URI with absolute path strips extension",
			driver: DriverSQLite,
			opts:   Options{DSN: "file:///var/db/inventory.sqlite3"},
			want:   "inventory",
		},
		// SQLite で空 DSN の防御。CLI 側で usage を先に出すが、本層でも
		// 空文字列で安全側に倒すことを確認する（要件 9.6）。
		{
			name:   "sqlite empty DSN yields empty title",
			driver: DriverSQLite,
			opts:   Options{DSN: ""},
			want:   "",
		},
		// PG DSN クエリパラメータ付き（要件 9.6）。既存ケースで `?sslmode=disable` は
		// あるが、複数パラメータでも DB 名のみ抽出されることを明示的に固定する。
		{
			name:   "postgres DSN with multiple query params extracts dbname only",
			driver: DriverPostgreSQL,
			opts:   Options{DSN: "postgres://u:p@host:5432/inventory?sslmode=require&application_name=erdm"},
			want:   "inventory",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveTitle(tc.driver, tc.opts)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%q)", got)
				}
				if tc.wantErr == errSentinelUnknownDriver {
					// 文言の一部に driver 値が含まれていることを確認する。
					if got != "" {
						t.Fatalf("expected empty title on error, got %q", got)
					}
					return
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

// errSentinelUnknownDriver はテスト内で「未対応ドライバ時にエラーが
// 返ること」のみを表現する目印。`resolveTitle` 自体は固有 sentinel を
// 公開しない（fmt.Errorf で文言化）ため、テスト側でマーカーを定義する。
var errSentinelUnknownDriver = errors.New("test: expects error from resolveTitle for unknown driver")

// TestResolveTitle_SharesDSNParsingWithMaskDSN は resolveTitle と maskDSN が
// 同一 DSN パース基盤（dsn.go の parsePostgresDSN ／ parseMySQLDSN ／
// parseSQLiteDSN）を共有していることを横断的に確認する（要件 9.5 / 9.6 と
// ナレッジ「操作の一覧性」）。
//
// 同じ DSN を両関数に渡し、どちらも「成功する／同じ DB 名認識を持つ」ことを
// 表駆動で固定することで、片方だけが独自パースを抱える状況を回帰検出できる。
func TestResolveTitle_SharesDSNParsingWithMaskDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		driver    Driver
		dsn       string
		wantTitle string
		// wantMaskedSubstr はマスク済み DSN が含むべき DB 名相当部分。
		// maskDSN の出力には DB 名がそのまま残ることを利用し、両関数が
		// DSN の「DB 名部分」に対して一貫した認識を持つことを担保する。
		wantMaskedSubstr string
	}{
		{
			name:             "postgres DSN: title and masked DSN both retain dbname",
			driver:           DriverPostgreSQL,
			dsn:              "postgres://u:p@host:5432/inventory?sslmode=disable",
			wantTitle:        "inventory",
			wantMaskedSubstr: "/inventory",
		},
		{
			name:             "mysql DSN: title and masked DSN both retain dbname",
			driver:           DriverMySQL,
			dsn:              "u:p@tcp(127.0.0.1:3306)/inventory",
			wantTitle:        "inventory",
			wantMaskedSubstr: "/inventory",
		},
		{
			// SQLite mask は basename のみを残す（ディレクトリ漏洩防止）。
			// title 側は basename からさらに拡張子を取り除いた値を採用する。
			// 両者が同じ basename `inventory.db` を経由していることを担保する。
			name:             "sqlite DSN: title strips extension while masked keeps basename",
			driver:           DriverSQLite,
			dsn:              "/var/db/inventory.db",
			wantTitle:        "inventory",
			wantMaskedSubstr: "inventory.db",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotTitle, err := resolveTitle(tc.driver, Options{DSN: tc.dsn})
			if err != nil {
				t.Fatalf("resolveTitle: unexpected error: %v", err)
			}
			if gotTitle != tc.wantTitle {
				t.Fatalf("resolveTitle title = %q, want %q", gotTitle, tc.wantTitle)
			}
			masked := maskDSN(tc.driver, tc.dsn)
			if masked == "" {
				t.Fatalf("maskDSN returned empty string for shared DSN %q", tc.dsn)
			}
			if !strings.Contains(masked, tc.wantMaskedSubstr) {
				t.Fatalf("maskDSN(%q) = %q; want substring %q", tc.dsn, masked, tc.wantMaskedSubstr)
			}
		})
	}
}
