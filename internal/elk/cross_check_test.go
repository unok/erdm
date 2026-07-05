// cross_check_test.go は Go `internal/elk` とフロント `frontend/src/layout/elk.ts`
// の ELK 入力グラフ「構造」の一致を担保するクロスチェックテスト（ADR-0001）。
//
// レイアウト計算そのものはブラウザ側（elkjs）が担うため一致対象にはしない。
// ここで縛るのは「一致すべき構造」= (1) primary グループ所属（groupNode 階層）と
// (2) エッジ集合（親→子、sanitizeID 正規化済み）である。
//
// 共有 fixture（`frontend/src/layout/testdata/elk-cross-check-fixtures.json`）を
// Go の `*model.Schema` にデコードし、buildRoot から構造サマリを抽出して期待値
// （`frontend/src/layout/testdata/expected/<name>.elk-structure.json`）と比較する。
// 期待値は Go 側を「真実の単一の源」とし、UPDATE_GOLDEN=1 で再生成できる。
// TS 側の cross-check.test.ts は同じ期待値ファイルへの一致を検証する。
package elk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/testutil/golden"
)

const (
	elkFixturePath = "../../frontend/src/layout/testdata/elk-cross-check-fixtures.json"
	elkExpectedDir = "../../frontend/src/layout/testdata/expected"
)

// elkStructureGroup は 1 つの primary グループとその所属テーブル ID 列。
type elkStructureGroup struct {
	ID     string   `json:"id"`
	Tables []string `json:"tables"`
}

// elkStructureEdge は 1 本の FK エッジ（親→子）。
type elkStructureEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// elkStructureSummary は ELK 入力グラフの「一致すべき構造」だけを取り出したもの。
// 座標・サイズ・layoutOptions は Go/TS で正当に異なるため含めない。
type elkStructureSummary struct {
	Groups    []elkStructureGroup `json:"groups"`
	Ungrouped []string            `json:"ungrouped"`
	Edges     []elkStructureEdge  `json:"edges"`
}

// summarizeRoot は elkRoot から構造サマリを抽出する。TS 側の同名処理と一致させる。
func summarizeRoot(root *elkRoot) elkStructureSummary {
	summary := elkStructureSummary{
		Groups:    []elkStructureGroup{},
		Ungrouped: []string{},
		Edges:     []elkStructureEdge{},
	}
	for _, child := range root.Children {
		if len(child.Children) > 0 {
			tables := make([]string, 0, len(child.Children))
			for _, member := range child.Children {
				tables = append(tables, member.ID)
			}
			summary.Groups = append(summary.Groups, elkStructureGroup{ID: child.ID, Tables: tables})
			continue
		}
		summary.Ungrouped = append(summary.Ungrouped, child.ID)
	}
	for _, e := range root.Edges {
		summary.Edges = append(summary.Edges, elkStructureEdge{Source: e.Sources[0], Target: e.Targets[0]})
	}
	return summary
}

func TestElkStructure_CrossCheckWithFrontend(t *testing.T) {
	raw, err := os.ReadFile(elkFixturePath)
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
			summary := summarizeRoot(buildRoot(schema))
			got, err := json.MarshalIndent(summary, "", "  ")
			if err != nil {
				t.Fatalf("marshal summary: %v", err)
			}
			got = append(got, '\n')
			golden.Compare(t, got, filepath.Join(elkExpectedDir, name+".elk-structure.json"))
		})
	}
}
