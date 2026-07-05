package model

import "strings"

// Table はスキーマ内の 1 テーブル定義。
//
// Name は物理名（識別子）、LogicalName は論理名。Columns は宣言順を保持し、
// PrimaryKeys は主キー構成カラムの Columns 内インデックス（宣言順）を保持する。
// Groups は所属するグループ名一覧で、Groups[0] が primary、それ以降が secondary。
// Groups が空（または nil）のテーブルは「グループ未指定」として扱う。
type Table struct {
	Name        string
	LogicalName string
	Columns     []Column
	PrimaryKeys []int
	Indexes     []Index
	Groups      []string
}

// PrimaryKeyColumnNames は主キー構成カラムの物理名をカンマ区切りで返す。
// 旧テンプレート参照名 {{.GetPrimaryKeyColumns}} の置換先
// （design.md §テンプレ対応表）。PrimaryKeys が空の場合は空文字列。
func (t *Table) PrimaryKeyColumnNames() string {
	if len(t.PrimaryKeys) == 0 {
		return ""
	}
	names := make([]string, 0, len(t.PrimaryKeys))
	for _, idx := range t.PrimaryKeys {
		// Schema.Validate で範囲外を弾く前提だが、ここでも防御する。
		if idx < 0 || idx >= len(t.Columns) {
			continue
		}
		names = append(names, t.Columns[idx].Name)
	}
	return strings.Join(names, ", ")
}

// PrimaryGroup は primary グループ名と存在フラグを返す。Groups の先頭要素が primary。
// グループ未指定（Groups が空）のときは ("", false) を返す。
func (t *Table) PrimaryGroup() (string, bool) {
	if len(t.Groups) == 0 {
		return "", false
	}
	return t.Groups[0], true
}

// SecondaryGroups は secondary グループ名の一覧を返す（Groups の 2 要素目以降）。
// secondary が無い場合は nil ではなく空スライスを返す（呼び出し側が range で
// 安全に扱えるように）。
func (t *Table) SecondaryGroups() []string {
	if len(t.Groups) <= 1 {
		return []string{}
	}
	out := make([]string, len(t.Groups)-1)
	copy(out, t.Groups[1:])
	return out
}
