package introspect

import (
	"context"
	"strings"
	"testing"
)

// TestIntrospect_RequiresDSN は DSN 空のとき防御的にエラーを返すことを確認する
// （要件 1.6 の防御層、CLI 側で先に弾く前提）。
func TestIntrospect_RequiresDSN(t *testing.T) {
	t.Parallel()
	_, err := Introspect(context.Background(), Options{})
	if err == nil {
		t.Fatalf("expected error for empty DSN, got nil")
	}
	if !strings.Contains(err.Error(), "dsn is required") {
		t.Fatalf("error %q should mention dsn is required", err.Error())
	}
}

// TestIntrospect_UnsupportedDriverOverride は --driver で未知の値を渡した場合に
// ドライバ確定段階でエラーが返ることを確認する（要件 2.4）。
//
// 接続前段階のため、原 DSN もエラー文言には埋め込まれない。
func TestIntrospect_UnsupportedDriverOverride(t *testing.T) {
	t.Parallel()
	opts := Options{
		Driver: Driver("oracle"),
		DSN:    "postgres://u:p@h:5432/db",
	}
	_, err := Introspect(context.Background(), opts)
	if err == nil {
		t.Fatalf("expected error for unsupported driver override, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("error %q should mention unsupported driver", err.Error())
	}
}

// TestIntrospect_DriverInferenceFailure は DSN プレフィックスからもファイル
// 拡張子からも推定できない場合にエラーが返ることを確認する（要件 1.5）。
func TestIntrospect_DriverInferenceFailure(t *testing.T) {
	t.Parallel()
	opts := Options{DSN: "no-prefix-no-extension"}
	_, err := Introspect(context.Background(), opts)
	if err == nil {
		t.Fatalf("expected driver inference failure, got nil")
	}
	if !strings.Contains(err.Error(), "cannot infer driver") {
		t.Fatalf("error %q should mention cannot infer driver", err.Error())
	}
}

// TestBuildKnownTables は内部 DTO 列から物理名集合を構築することを確認する。
func TestBuildKnownTables(t *testing.T) {
	t.Parallel()
	raws := []rawTable{
		{Name: "users"},
		{Name: "orders"},
	}
	known := buildKnownTables(raws)
	if _, ok := known["users"]; !ok {
		t.Errorf("known should contain users")
	}
	if _, ok := known["orders"]; !ok {
		t.Errorf("known should contain orders")
	}
	if _, ok := known["missing"]; ok {
		t.Errorf("known should not contain missing")
	}
}

// TestOpenDB_UnsupportedDriver は driverNames に登録されていないドライバ値で
// openDB を呼んだ場合にエラーが返ることを確認する。
func TestOpenDB_UnsupportedDriver(t *testing.T) {
	t.Parallel()
	_, err := openDB(Driver("oracle"), "anything")
	if err == nil {
		t.Fatalf("expected error for unsupported driver, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("error %q should mention unsupported driver", err.Error())
	}
}
