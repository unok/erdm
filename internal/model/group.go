package model

// Group はテーブルが所属する論理的なグループを表すプレゼンテーション派生値。
//
// Schema は Group 集約を持たず、各テーブルが Groups []string を保持する。
// レンダラ層が cluster 化の単位として扱う場合は DeriveGroups で Schema から
// 派生させる。Tables は当該グループに primary 所属するテーブル物理名一覧
// （登場順を保持）。
type Group struct {
	Name   string
	Tables []string
}

// DeriveGroups は Schema から登場順を保持した primary グループ集合を派生させる。
//
// 派生規則:
//   - Schema.Groups の登場順（パース時に最初に出現した順）をそのまま採用する。
//   - 各 Group.Tables には、その Group を primary（Groups[0]）として宣言した
//     Table の Name を Schema.Tables の登場順で詰める。
//   - secondary でしか所属しないテーブルは Group.Tables に含めない（DOT cluster
//     は primary のみで構築する設計に合わせるため。要件 2.10〜2.12）。
//   - Schema.Groups に存在するが primary 所属テーブルがいないグループも
//     空 Tables のまま返す（登場順保持の不変条件を優先）。
func DeriveGroups(s *Schema) []Group {
	if s == nil || len(s.Groups) == 0 {
		return []Group{}
	}
	out := make([]Group, 0, len(s.Groups))
	for _, name := range s.Groups {
		g := Group{Name: name, Tables: []string{}}
		for ti := range s.Tables {
			t := &s.Tables[ti]
			primary, ok := t.PrimaryGroup()
			if !ok || primary != name {
				continue
			}
			g.Tables = append(g.Tables, t.Name)
		}
		out = append(out, g)
	}
	return out
}
