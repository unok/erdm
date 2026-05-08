// cross_check_test.go は Go/TS シリアライザ出力のクロスチェックテスト。
//
// Requirements: 7.10
package serializer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/unok/erdm/internal/model"
)

// TestSerialize_CrossCheckWithFrontend は SPA 側の TS シリアライザと
// バイト一致（要件 7.10）を担保するため、共有 fixture
// (`frontend/src/serializer/testdata/cross-check-fixtures.json`) を Go の
// `*model.Schema` にデコードし、Serialize の出力が期待値ファイル
// (`frontend/src/serializer/testdata/expected/<name>.erdm`) とバイト単位で
// 一致することを確認する。
//
// 期待値ファイルは Go 側 Serialize の出力を「真実の単一の源」として扱う。
// TS 側はこの期待値とバイト一致することをクロスチェックテストで確認する。
//
// 期待値が古くなった場合は本テストが先に失敗するため、Go 側の修正と期待値
// 更新を同じコミットで行うこと。
func TestSerialize_CrossCheckWithFrontend(t *testing.T) {
	const fixturePath = "../../frontend/src/serializer/testdata/cross-check-fixtures.json"
	const expectedDir = "../../frontend/src/serializer/testdata/expected"

	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures map[string]*model.Schema
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("unmarshal fixtures: %v", err)
	}

	names := make([]string, 0, len(fixtures))
	for name := range fixtures {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			schema := fixtures[name]
			if schema == nil {
				t.Fatalf("fixture %s missing", name)
			}
			got, err := Serialize(schema)
			if err != nil {
				t.Fatalf("Serialize(%s): %v", name, err)
			}
			expectedPath := filepath.Join(expectedDir, name+".erdm")
			want, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read expected %s: %v", expectedPath, err)
			}
			if string(got) != string(want) {
				t.Errorf("cross-check mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					name, string(got), string(want))
			}
		})
	}
}
