// layout_test.go は internal/layout パッケージのユニットテスト。
//
// Requirements: 6.1, 6.2, 6.5, 6.6, 10.3
package layout

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// TestLoad_NonExistent_ReturnsEmpty は存在しないパスを Load した場合、
// 空 Layout と nil（LoadError 無し）が返ることを確認する（要件 6.5）。
func TestLoad_NonExistent_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no_such_layout.json")

	got, loadErr := Load(missing)
	if loadErr != nil {
		t.Fatalf("Load(missing) should not return LoadError, got: %v", loadErr)
	}
	if got == nil {
		t.Fatalf("Load(missing) should return non-nil empty Layout")
	}
	if len(got) != 0 {
		t.Fatalf("Load(missing) should return empty Layout, got %d entries", len(got))
	}
}

// TestLoad_ValidJSON は正常な JSON ファイルから読み込み、期待値と一致することを確認する（要件 6.1）。
func TestLoad_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.json")
	content := []byte(`{"users":{"x":10.5,"y":20.25},"orders":{"x":-5,"y":7.0}}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, loadErr := Load(path)
	if loadErr != nil {
		t.Fatalf("Load(valid) returned LoadError: %v", loadErr)
	}
	if len(got) != 2 {
		t.Fatalf("Load returned %d entries, want 2", len(got))
	}
	if got["users"] != (Position{X: 10.5, Y: 20.25}) {
		t.Fatalf("users position = %+v, want {10.5, 20.25}", got["users"])
	}
	if got["orders"] != (Position{X: -5, Y: 7.0}) {
		t.Fatalf("orders position = %+v, want {-5, 7.0}", got["orders"])
	}
}

// TestLoad_CorruptedJSON_ReturnsLoadError は破損 JSON で *LoadError が
// 返り、`Cause` に JSON エラー情報が含まれることを確認する（要件 6.6）。
func TestLoad_CorruptedJSON_ReturnsLoadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, loadErr := Load(path)
	if loadErr == nil {
		t.Fatalf("Load(broken) should return *LoadError, got nil; layout=%v", got)
	}
	if loadErr.Path != path {
		t.Fatalf("LoadError.Path = %q, want %q", loadErr.Path, path)
	}
	if loadErr.Cause == "" {
		t.Fatalf("LoadError.Cause should not be empty")
	}
	// errors.As で型判別できることを確認（呼び出し側で 500 マッピングなどに使う想定）
	var le *LoadError
	if !errors.As(error(loadErr), &le) {
		t.Fatalf("LoadError should satisfy errors.As targeting *LoadError")
	}
}

// TestLoad_PermissionDenied_ReturnsLoadError は読み取り不可ファイルで
// *LoadError が返ることを確認する（要件 6.6）。Windows ではファイル権限
// モデルが POSIX と異なるためスキップする。
func TestLoad_PermissionDenied_ReturnsLoadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "noread.json")
	if err := os.WriteFile(path, []byte(`{"a":{"x":1,"y":2}}`), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod 0000: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	got, loadErr := Load(path)
	if loadErr == nil {
		t.Fatalf("Load(unreadable) should return *LoadError, got nil; layout=%v", got)
	}
	if loadErr.Path != path {
		t.Fatalf("LoadError.Path = %q, want %q", loadErr.Path, path)
	}
}

// TestSave_AtomicReplace は Save 後にファイル内容が新値で置換されている
// ことを確認する（要件 6.2 / 10.3）。
func TestSave_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.json")
	if err := os.WriteFile(path, []byte(`{"old":{"x":0,"y":0}}`), 0644); err != nil {
		t.Fatalf("write original: %v", err)
	}

	newLayout := Layout{"users": {X: 100, Y: 200}}
	if err := Save(path, newLayout); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after Save: %v", err)
	}
	var got Layout
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Save output is not valid JSON: %v\nraw=%s", err, data)
	}
	if _, exists := got["old"]; exists {
		t.Fatalf("old entry should be replaced, got: %v", got)
	}
	if got["users"] != (Position{X: 100, Y: 200}) {
		t.Fatalf("users = %+v, want {100, 200}", got["users"])
	}
}

// TestSave_RoundTrip は Save → Load の往復で値が完全一致することを確認する（要件 6.1 / 6.2）。
func TestSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rt.json")

	want := Layout{
		"users":  {X: 1.5, Y: 2.5},
		"orders": {X: -3.0, Y: 4.0},
		"items":  {X: 0, Y: 0},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, loadErr := Load(path)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if len(got) != len(want) {
		t.Fatalf("Load returned %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("entry %q = %+v, want %+v", k, got[k], v)
		}
	}
}

// TestSave_FailureLeavesOriginalIntact は書き込み失敗時に元ファイルが
// 破壊されないことを確認する（要件 10.3）。ディレクトリへの書き込み権限を
// 剥奪し、`os.CreateTemp` を失敗させる。Windows ではスキップ。
func TestSave_FailureLeavesOriginalIntact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.json")
	original := []byte(`{"intact":{"x":1,"y":2}}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod dir 0500: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	err := Save(path, Layout{"new": {X: 9, Y: 9}})
	if err == nil {
		t.Fatalf("Save should fail when temp file cannot be created")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("original file unreadable after failed Save: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("original file modified after failed Save\nwant %s\ngot  %s", original, got)
	}
}

// TestSave_ConcurrentWrites_NoCorruption は複数 goroutine が同じ path に
// 並行で Save を行っても、最終ファイルが JSON として有効であり（部分書き
// 込みによる破損が起きない）、いずれかの書き込み内容と完全一致することを
// 確認する（要件 10.3）。
func TestSave_ConcurrentWrites_NoCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.json")
	if err := Save(path, Layout{"seed": {X: 0, Y: 0}}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	const writers = 10
	candidates := make([]Layout, writers)
	for i := 0; i < writers; i++ {
		candidates[i] = Layout{
			"writer": {X: float64(i), Y: float64(i * 2)},
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := Save(path, candidates[idx]); err != nil {
				t.Errorf("Save #%d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	got, loadErr := Load(path)
	if loadErr != nil {
		t.Fatalf("Load after concurrent Save returned LoadError: %v", loadErr)
	}
	if len(got) != 1 {
		t.Fatalf("final layout has %d entries, want 1", len(got))
	}
	pos, ok := got["writer"]
	if !ok {
		t.Fatalf("final layout missing 'writer' key: %v", got)
	}
	matched := false
	for _, c := range candidates {
		if c["writer"] == pos {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("final position %+v does not match any writer candidate", pos)
	}
}

// TestPosition_JSONFieldNames は Position の JSON タグが小文字 `x` / `y` で
// あることを確認する（design.md §論理データモデル整合）。
func TestPosition_JSONFieldNames(t *testing.T) {
	p := Position{X: 1, Y: 2}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)
	want := `{"x":1,"y":2}`
	if got != want {
		t.Fatalf("Position JSON = %s, want %s", got, want)
	}
}

// TestLoadError_ErrorMessage は LoadError.Error() が Path と Cause を
// 含む文字列を返すことを確認する。
func TestLoadError_ErrorMessage(t *testing.T) {
	le := &LoadError{Path: "/tmp/x.json", Cause: "boom"}
	msg := le.Error()
	if msg == "" {
		t.Fatalf("Error() should not be empty")
	}
	for _, want := range []string{"/tmp/x.json", "boom"} {
		if !contains(msg, want) {
			t.Fatalf("Error() = %q, should contain %q", msg, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
