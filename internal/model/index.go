package model

import "strings"

// Index はテーブルに付随するインデックス定義。
//
// Name はインデックス物理名、Columns はインデックス対象のカラム物理名一覧
// （宣言順を保持）、IsUnique は UNIQUE 指定の有無。
type Index struct {
	Name     string
	Columns  []string
	IsUnique bool
}

// ColumnNames はインデックス構成カラム名をカンマ区切りで返す。
// 旧テンプレート参照名 {{.GetIndexColumns}} の置換先（design.md §テンプレ対応表）。
func (i *Index) ColumnNames() string {
	return strings.Join(i.Columns, ", ")
}
