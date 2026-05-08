// Package fixtures はテストから既存サンプル `.erdm` を一次資料経由で読み込む
// 共通導線を提供する。
//
// 配置方針:
//
//	サンプル `.erdm` は doc/sample/ を一次資料として保持し、各パッケージの
//	testdata/ には複製しない。テストは LoadFixture(name) を呼び出して読み込む。
//
// 解決ロジック:
//
//	テストはパッケージ作業ディレクトリ（go test の cwd）から実行される。
//	呼び出し元から見たリポジトリルートまでの相対距離が異なるため、
//	呼び出し元の cwd から親方向に向かって `doc/sample/` ディレクトリを探索し、
//	見つかった時点で読み取る。リポジトリ外で実行される想定はない。
package fixtures

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SampleNames は doc/sample/ 配下に置かれたサンプル `.erdm` の論理名一覧。
// テストが網羅したいとき NamesAll() でも参照できる。
var SampleNames = []string{
	"test",
	"test_jp",
	"test_large_data_jp",
	"test_no_logical_name",
}

// NamesAll は SampleNames のコピーを返す（呼び出し側の改変を防ぐため）。
func NamesAll() []string {
	out := make([]string, len(SampleNames))
	copy(out, SampleNames)
	return out
}

// LoadFixture は doc/sample/<name>.erdm を読み込んで内容を返す。
// 見つからなければエラー。テスト本体は require.NoError 等で受ければよい。
func LoadFixture(name string) ([]byte, error) {
	root, err := findRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("fixtures: locate repo root: %w", err)
	}
	path := filepath.Join(root, "doc", "sample", name+".erdm")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fixtures: read %s: %w", path, err)
	}
	return data, nil
}

// FixturePath は doc/sample/<name>.erdm の絶対パスを返す。テストが Compare の
// path を組み立てる用途。
func FixturePath(name string) (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", fmt.Errorf("fixtures: locate repo root: %w", err)
	}
	return filepath.Join(root, "doc", "sample", name+".erdm"), nil
}

// findRepoRoot はカレントディレクトリから親方向に上がりつつ
// `doc/sample` ディレクトリを含む祖先を探す。go.mod を併用して
// リポジトリ境界を確認する。
func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if hasRepoMarkers(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root not found (missing doc/sample and go.mod)")
		}
		dir = parent
	}
}

// hasRepoMarkers は dir がリポジトリルートかどうか判定する。
// doc/sample ディレクトリと go.mod の両方が存在することを条件とする。
func hasRepoMarkers(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return false
	}
	if st, err := os.Stat(filepath.Join(dir, "doc", "sample")); err != nil || !st.IsDir() {
		return false
	}
	return true
}
