package model

import (
	"reflect"
	"testing"
)

func TestTable_PrimaryKeyColumnNames(t *testing.T) {
	tbl := Table{
		Name: "users",
		Columns: []Column{
			{Name: "id"},
			{Name: "tenant_id"},
			{Name: "email"},
		},
		PrimaryKeys: []int{0, 1},
	}
	got := tbl.PrimaryKeyColumnNames()
	want := "id, tenant_id"
	if got != want {
		t.Fatalf("PrimaryKeyColumnNames()=%q want %q", got, want)
	}
}

func TestTable_PrimaryKeyColumnNames_Empty(t *testing.T) {
	tbl := Table{Name: "users", Columns: []Column{{Name: "id"}}}
	if got := tbl.PrimaryKeyColumnNames(); got != "" {
		t.Fatalf("PrimaryKeyColumnNames()=%q want empty", got)
	}
}

func TestTable_PrimaryKeyColumnNames_OutOfRangeSkipped(t *testing.T) {
	tbl := Table{
		Name:        "t",
		Columns:     []Column{{Name: "id"}},
		PrimaryKeys: []int{0, 5}, // 5 は範囲外。Validate なら違反だが派生計算では握りつぶさず安全側で省略。
	}
	got := tbl.PrimaryKeyColumnNames()
	want := "id"
	if got != want {
		t.Fatalf("PrimaryKeyColumnNames()=%q want %q", got, want)
	}
}

func TestTable_PrimaryGroup(t *testing.T) {
	cases := []struct {
		name     string
		groups   []string
		wantName string
		wantOK   bool
	}{
		{"nil", nil, "", false},
		{"empty", []string{}, "", false},
		{"single", []string{"core"}, "core", true},
		{"multiple", []string{"core", "audit"}, "core", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tbl := Table{Groups: tc.groups}
			gotName, gotOK := tbl.PrimaryGroup()
			if gotName != tc.wantName || gotOK != tc.wantOK {
				t.Fatalf("PrimaryGroup()=(%q,%v) want (%q,%v)", gotName, gotOK, tc.wantName, tc.wantOK)
			}
		})
	}
}

func TestTable_SecondaryGroups(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
		want   []string
	}{
		{"nil", nil, []string{}},
		{"single", []string{"core"}, []string{}},
		{"two", []string{"core", "audit"}, []string{"audit"}},
		{"three", []string{"core", "audit", "ext"}, []string{"audit", "ext"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tbl := Table{Groups: tc.groups}
			got := tbl.SecondaryGroups()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SecondaryGroups()=%#v want %#v", got, tc.want)
			}
		})
	}
}

func TestTable_SecondaryGroups_NotAlias(t *testing.T) {
	// 返却スライスが内部スライスのエイリアスではないこと（呼び出し側の改変が
	// 元データに波及しないこと）を保証する。
	tbl := Table{Groups: []string{"core", "audit", "ext"}}
	got := tbl.SecondaryGroups()
	got[0] = "MUTATED"
	if tbl.Groups[1] != "audit" {
		t.Fatalf("SecondaryGroups must not alias internal slice; tbl.Groups[1]=%q", tbl.Groups[1])
	}
}
