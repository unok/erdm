package introspect

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestParseMySQLDSN_RejectsURLScheme はタスク 3.2 で採用した境界仕様
// （`mysql://` URL は接続段階で明示エラー）が単一窓口（parseMySQLDSN）に
// 集約されていることを契約として固定する（要件 1.5 / 2.2 / 11.2）。
func TestParseMySQLDSN_RejectsURLScheme(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dsn  string
	}{
		{name: "lowercase mysql://", dsn: "mysql://user:pass@host:3306/db"},
		{name: "uppercase MYSQL://", dsn: "MYSQL://user:pass@host:3306/db"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := parseMySQLDSN(tc.dsn)
			if err == nil {
				t.Fatalf("expected error for %q, got cfg=%+v", tc.dsn, cfg)
			}
			if !errors.Is(err, errMySQLURLSchemeNotSupported) {
				t.Fatalf("err = %v, want errors.Is(errMySQLURLSchemeNotSupported)", err)
			}
		})
	}
}

// TestParseMySQLDSN_AcceptsStandardForm は標準形式 DSN（go-sql-driver/mysql の
// 既定 DSN 文法）が共有窓口でそのまま解析できることを固定する（タスク 3.2）。
func TestParseMySQLDSN_AcceptsStandardForm(t *testing.T) {
	t.Parallel()

	cfg, err := parseMySQLDSN("user:secret@tcp(127.0.0.1:3306)/shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.User != "user" {
		t.Fatalf("User = %q, want %q", cfg.User, "user")
	}
	if cfg.Passwd != "secret" {
		t.Fatalf("Passwd = %q, want %q", cfg.Passwd, "secret")
	}
	if cfg.DBName != "shop" {
		t.Fatalf("DBName = %q, want %q", cfg.DBName, "shop")
	}
}

// TestParseMySQLDSN_RejectsURLScheme_MessageMentionsStandardForm は
// `mysql://` URL DSN を拒否する際のエラーメッセージが
// 「standard DSN への置換」を促す表現を含むことを契約として固定する
// （要件 11.2「利用者にとって何が起きたかが一目でわかる」）。
//
// メッセージ全文一致ではなく主要キーワード（"mysql://"・"tcp"）の含有で
// 確認することで、文言の細かな調整に対する脆さを避ける（テストポリシー
// "メッセージは Contains で確認"）。
func TestParseMySQLDSN_RejectsURLScheme_MessageMentionsStandardForm(t *testing.T) {
	t.Parallel()

	_, err := parseMySQLDSN("mysql://user:secret@host:3306/db")
	if err == nil {
		t.Fatalf("expected error for mysql:// URL DSN")
	}
	msg := err.Error()
	if !strings.Contains(msg, "mysql://") {
		t.Errorf("error %q should mention mysql:// to identify the offending scheme", msg)
	}
	if !strings.Contains(msg, "tcp(") {
		t.Errorf("error %q should hint at standard DSN form 'tcp(host:port)'", msg)
	}
}

// TestMaskDSN_MySQLURLSchemeReturnsEmpty_LeaksNoOriginalDSN は mysql:// URL
// に対して maskDSN が空文字列を返し、原 DSN を漏らさないことを固定する
// （要件 10.4：機密情報の漏洩防止）。
func TestMaskDSN_MySQLURLSchemeReturnsEmpty_LeaksNoOriginalDSN(t *testing.T) {
	t.Parallel()

	const original = "mysql://admin:S3cret@dbhost:3306/inventory"
	got := maskDSN(DriverMySQL, original)
	if got != "" {
		t.Fatalf("maskDSN should return empty string for mysql:// URL, got %q", got)
	}
	// 原 DSN（特にパスワード "S3cret"）が戻り値に含まれないことを念のため確認。
	if strings.Contains(got, "S3cret") {
		t.Fatalf("masked output leaked original password: %q", got)
	}
}

// TestResolveTitle_MySQLURLSchemePropagatesSentinel は mysql:// URL を
// `resolveTitle(DriverMySQL, ...)` に渡したときに `errMySQLURLSchemeNotSupported`
// が呼び出し側へ伝播することを固定する（要件 1.5 / 2.2 / 11.2）。
//
// 既存 TestResolveTitle 表内に同等ケースは存在するが、本テストは「DSN 解析の
// 単一窓口（parseMySQLDSN）が境界エラーを共通伝播させる」ことを契約として
// 明示する責務を持つ。
func TestResolveTitle_MySQLURLSchemePropagatesSentinel(t *testing.T) {
	t.Parallel()

	_, err := resolveTitle(DriverMySQL, Options{DSN: "mysql://user:pass@host:3306/db"})
	if err == nil {
		t.Fatalf("expected error for mysql:// URL DSN")
	}
	if !errors.Is(err, errMySQLURLSchemeNotSupported) {
		t.Fatalf("err = %v, want errors.Is(errMySQLURLSchemeNotSupported)", err)
	}
}

// TestIntrospect_MySQLURLSchemeFailsBeforeConnect は Introspect 公開関数に
// `mysql://` URL DSN を渡したとき、接続段階に達する前（タイトル解決段階）で
// `errMySQLURLSchemeNotSupported` が返ることを確認する（要件 1.5 / 2.2 / 11.2）。
//
// 推定段階では `detectDriver` が `mysql://` を MySQL として識別する一方
// （要件 1.5）、接続前段で同 sentinel を投げる境界仕様（タスク 3.2 採用方針）が
// パイプライン全体で機能していることの回帰検出。
func TestIntrospect_MySQLURLSchemeFailsBeforeConnect(t *testing.T) {
	t.Parallel()

	_, err := Introspect(context.Background(), Options{DSN: "mysql://user:secret@dbhost:3306/inventory"})
	if err == nil {
		t.Fatalf("expected error for mysql:// URL DSN")
	}
	if !errors.Is(err, errMySQLURLSchemeNotSupported) {
		t.Fatalf("err = %v, want errors.Is(errMySQLURLSchemeNotSupported)", err)
	}
	// 原 DSN のパスワード "secret" がエラー文言に含まれないことを確認
	// （要件 10.4：maskDSN を経由して原 DSN を漏らさない）。
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error message leaked password: %v", err)
	}
}

// TestParseMySQLDSN_AcceptsStandardForm_WithQueryParams は標準形式 DSN の
// クエリパラメータ部分も正しくパースされることを固定する（要件 1.5 の
// 「標準形式は受理」契約の補強）。
//
// go-sql-driver/mysql では `parseTime` のような既知パラメータは Config の
// 専用フィールドに格納され、未知の独自パラメータは `Params` マップに残る。
// 本テストは両方の経路を 1 件ずつ押さえる。
func TestParseMySQLDSN_AcceptsStandardForm_WithQueryParams(t *testing.T) {
	t.Parallel()

	cfg, err := parseMySQLDSN("user:secret@tcp(127.0.0.1:3306)/shop?parseTime=true&customParam=42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBName != "shop" {
		t.Fatalf("DBName = %q, want %q", cfg.DBName, "shop")
	}
	if !cfg.ParseTime {
		t.Errorf("ParseTime = false; want true (known param routed to dedicated field)")
	}
	if cfg.Params["customParam"] != "42" {
		t.Errorf("Params[customParam] = %q; want %q (custom param routed to Params map)",
			cfg.Params["customParam"], "42")
	}
}

// TestParseSQLiteDSN は file: URI とプレーンパスのいずれもファイルパスを
// 抽出できることを固定する（タスク 3.4 のタイトル既定値解決の前提条件）。
func TestParseSQLiteDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{name: "plain path", dsn: "./var/shop.db", want: "./var/shop.db"},
		{name: "file URI without authority", dsn: "file:./var/shop.db?cache=shared", want: "./var/shop.db"},
		{name: "file URI with absolute path", dsn: "file:///abs/path/shop.db", want: "/abs/path/shop.db"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSQLiteDSN(tc.dsn)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("path = %q, want %q", got, tc.want)
			}
		})
	}
}
