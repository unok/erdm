// Package golden provides minimal helpers for golden-file based testing.
//
// 配置規約:
//
//	各パッケージの testdata/golden/<case>.golden に固定された期待出力を置く。
//	テストは Compare(t, got, "testdata/golden/<case>.golden") を呼び出して比較する。
//
// 更新フラグ:
//
//	環境変数 UPDATE_GOLDEN=1 が設定されているときのみ、不一致の golden を実値で
//	書き換える。それ以外では不一致を t.Fatalf で fail させる。
//
//	CI 環境（環境変数 CI=true）では UPDATE_GOLDEN=1 を同時に許可しない。
//	両方が同時に有効な場合、Compare は即時 fail する。これは CI でのゴールデン
//	誤更新の事故を防ぐためのガード。
package golden

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateFlag は -update フラグでも UPDATE_GOLDEN を有効化できるようにする。
// 環境変数 UPDATE_GOLDEN=1 と OR で結合する（どちらかが立てば更新モード）。
var updateFlag = flag.Bool("update", false, "rewrite golden files with the actual output")

// Update は呼び出し時点での更新モードを返す。
//
// 環境変数 UPDATE_GOLDEN=1/true もしくは -update フラグが立っているときに true。
// CI=true と UPDATE_GOLDEN=1 が同時に有効な場合は呼び出し側で検出して fail させる。
func Update() bool {
	if *updateFlag {
		return true
	}
	v := os.Getenv("UPDATE_GOLDEN")
	return strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
}

// guardCIMisuse は CI 環境で UPDATE_GOLDEN が誤って有効化されていないか検査する。
func guardCIMisuse() error {
	v := os.Getenv("CI")
	ci := strings.EqualFold(v, "true") || v == "1"
	if ci && Update() {
		return errors.New("golden: UPDATE_GOLDEN/-update must not be enabled when CI=true (refusing to overwrite goldens in CI)")
	}
	return nil
}

// resultKind は compareDecide の判定種別。
type resultKind int

const (
	resultMatch resultKind = iota
	resultUpdated
	resultMismatch
	resultMissing
	resultRefusedInCI
	resultIOError
)

// compareResult は compareDecide の戻り値。Compare 本体が t.Fatalf に変換する。
type compareResult struct {
	kind    resultKind
	message string
}

// compareDecide は got と path のゴールデンファイルを比較し、結果と必要な副作用
// （更新時の書き出し）を行ったうえで戻り値を返す。t に依存しないので単体テスト
// しやすい。
func compareDecide(got []byte, path string) compareResult {
	if err := guardCIMisuse(); err != nil {
		return compareResult{kind: resultRefusedInCI, message: err.Error()}
	}

	if Update() {
		if err := writeGolden(path, got); err != nil {
			return compareResult{kind: resultIOError, message: fmt.Sprintf("golden: failed to update %s: %v", path, err)}
		}
		return compareResult{kind: resultUpdated, message: fmt.Sprintf("golden: updated %s (%d bytes)", path, len(got))}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return compareResult{
				kind:    resultMissing,
				message: fmt.Sprintf("golden: %s does not exist; rerun with UPDATE_GOLDEN=1 (or -update) to create it", path),
			}
		}
		return compareResult{kind: resultIOError, message: fmt.Sprintf("golden: failed to read %s: %v", path, err)}
	}

	if bytes.Equal(want, got) {
		return compareResult{kind: resultMatch}
	}

	return compareResult{
		kind:    resultMismatch,
		message: fmt.Sprintf("golden mismatch: %s\n%s", path, summarizeDiff(want, got)),
	}
}

// Compare は got とゴールデンファイル path の中身を比較する。
//
// 一致: 何もしない。
// 不一致 + Update()==true: path に got を書き出して成功扱い。親ディレクトリが
//
//	なければ作成する。
//
// 不一致 + Update()==false: t.Fatalf で diff サマリと共に fail する。
// path が存在しない + Update()==true: 新規にゴールデンを作成する。
// path が存在しない + Update()==false: t.Fatalf で「ゴールデンが無い」旨を出す。
//
// path は呼び出し側パッケージのテストファイル位置からの相対パス、または絶対パス。
// パッケージ作業ディレクトリ起点で解決されるため、`testdata/golden/foo.golden`
// の形を推奨する。
func Compare(t *testing.T, got []byte, path string) {
	t.Helper()
	r := compareDecide(got, path)
	switch r.kind {
	case resultMatch:
		return
	case resultUpdated:
		t.Log(r.message)
	case resultMismatch, resultMissing, resultRefusedInCI, resultIOError:
		t.Fatal(r.message)
	default:
		t.Fatalf("golden: unknown result kind: %v", r.kind)
	}
}

// writeGolden は got を path に書き出す。親ディレクトリが無ければ作る。
func writeGolden(path string, got []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, got, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// summarizeDiff は want と got の最初の不一致箇所を要約して返す。
// 大きなファイルでもログを汚さない短い表現にとどめる。
func summarizeDiff(want, got []byte) string {
	const maxPreview = 200
	first := firstDiffOffset(want, got)
	wantSnippet := snippetAround(want, first, maxPreview)
	gotSnippet := snippetAround(got, first, maxPreview)
	return fmt.Sprintf(
		"  size: want=%d got=%d\n"+
			"  first diff at byte %d\n"+
			"  want: %q\n"+
			"  got:  %q\n"+
			"  hint: rerun with UPDATE_GOLDEN=1 (or -update) after confirming the new output is intended",
		len(want), len(got), first, wantSnippet, gotSnippet,
	)
}

// firstDiffOffset は最初に値が異なるバイト位置を返す。等しい場合は短い方の長さを返す。
func firstDiffOffset(want, got []byte) int {
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if want[i] != got[i] {
			return i
		}
	}
	return n
}

// snippetAround は offset 付近の最大 limit バイトを切り出す。
func snippetAround(buf []byte, offset, limit int) string {
	if len(buf) == 0 {
		return ""
	}
	start := offset - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(buf) {
		end = len(buf)
	}
	return string(buf[start:end])
}
