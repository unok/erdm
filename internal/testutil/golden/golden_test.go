package golden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdate_FlagAndEnv(t *testing.T) {
	t.Setenv("UPDATE_GOLDEN", "")
	if Update() {
		t.Fatalf("Update() should be false when neither flag nor env is set")
	}

	t.Setenv("UPDATE_GOLDEN", "1")
	if !Update() {
		t.Fatalf("Update() should be true when UPDATE_GOLDEN=1")
	}

	t.Setenv("UPDATE_GOLDEN", "true")
	if !Update() {
		t.Fatalf("Update() should be true when UPDATE_GOLDEN=true")
	}

	t.Setenv("UPDATE_GOLDEN", "0")
	if Update() {
		t.Fatalf("Update() should be false when UPDATE_GOLDEN=0")
	}
}

func TestGuardCIMisuse(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("UPDATE_GOLDEN", "1")
	if err := guardCIMisuse(); err == nil {
		t.Fatalf("guardCIMisuse should fail when CI=true and UPDATE_GOLDEN=1")
	}

	t.Setenv("CI", "true")
	t.Setenv("UPDATE_GOLDEN", "")
	if err := guardCIMisuse(); err != nil {
		t.Fatalf("guardCIMisuse should pass when CI=true alone: %v", err)
	}

	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "1")
	if err := guardCIMisuse(); err != nil {
		t.Fatalf("guardCIMisuse should pass when UPDATE_GOLDEN=1 alone: %v", err)
	}
}

func TestCompareDecide_Match(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "ok.golden")
	want := []byte("hello\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := compareDecide(want, path)
	if got.kind != resultMatch {
		t.Fatalf("expected resultMatch, got kind=%v message=%q", got.kind, got.message)
	}
}

func TestCompareDecide_Mismatch(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "diff.golden")
	if err := os.WriteFile(path, []byte("expected\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got := compareDecide([]byte("actual\n"), path)
	if got.kind != resultMismatch {
		t.Fatalf("expected resultMismatch, got kind=%v message=%q", got.kind, got.message)
	}
	if got.message == "" {
		t.Fatalf("mismatch message should not be empty")
	}
}

func TestCompareDecide_UpdateWritesFile(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "1")

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.golden")

	got := []byte("fresh\n")
	res := compareDecide(got, path)
	if res.kind != resultUpdated {
		t.Fatalf("expected resultUpdated, got kind=%v message=%q", res.kind, res.message)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to be written: %v", err)
	}
	if string(written) != string(got) {
		t.Fatalf("written content mismatch: want %q got %q", string(got), string(written))
	}
}

func TestCompareDecide_UpdateOverwritesMismatch(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "1")

	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.golden")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	res := compareDecide([]byte("new\n"), path)
	if res.kind != resultUpdated {
		t.Fatalf("expected resultUpdated, got kind=%v", res.kind)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(written) != "new\n" {
		t.Fatalf("expected golden to be overwritten with new content, got %q", string(written))
	}
}

func TestCompareDecide_Missing(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "missing.golden")

	res := compareDecide([]byte("anything"), path)
	if res.kind != resultMissing {
		t.Fatalf("expected resultMissing, got kind=%v message=%q", res.kind, res.message)
	}
}

func TestCompareDecide_RefuseInCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("UPDATE_GOLDEN", "1")

	dir := t.TempDir()
	path := filepath.Join(dir, "ci.golden")

	res := compareDecide([]byte("anything"), path)
	if res.kind != resultRefusedInCI {
		t.Fatalf("expected resultRefusedInCI, got kind=%v message=%q", res.kind, res.message)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should not have been written under CI guard")
	}
}

// TestCompare_HappyPath は公開 API Compare が一致時に t.Fatalf しないことを確認する。
func TestCompare_HappyPath(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("UPDATE_GOLDEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "hp.golden")
	body := []byte("ok\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	Compare(t, body, path)
}

func TestSummarizeDiff_NonEmpty(t *testing.T) {
	out := summarizeDiff([]byte("aaa"), []byte("aab"))
	if out == "" {
		t.Fatalf("summarizeDiff should produce non-empty output")
	}
}

func TestFirstDiffOffset(t *testing.T) {
	cases := []struct {
		name string
		a, b []byte
		want int
	}{
		{"equal", []byte("abc"), []byte("abc"), 3},
		{"diff at 0", []byte("abc"), []byte("xbc"), 0},
		{"diff at 2", []byte("abc"), []byte("abx"), 2},
		{"shorter b", []byte("abc"), []byte("ab"), 2},
		{"shorter a", []byte("ab"), []byte("abc"), 2},
		{"both empty", []byte{}, []byte{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstDiffOffset(c.a, c.b); got != c.want {
				t.Fatalf("want %d got %d", c.want, got)
			}
		})
	}
}
