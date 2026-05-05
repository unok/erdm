package model

import (
	"reflect"
	"testing"
)

func TestDeriveGroups_OrderPreserved(t *testing.T) {
	s := &Schema{
		Title:  "test",
		Groups: []string{"core", "audit", "ext"},
		Tables: []Table{
			{Name: "users", Groups: []string{"core"}},
			{Name: "logs", Groups: []string{"audit", "core"}}, // primary は audit
			{Name: "ext_setting", Groups: []string{"ext"}},
			{Name: "tags"}, // ungrouped
		},
	}
	got := DeriveGroups(s)
	want := []Group{
		{Name: "core", Tables: []string{"users"}},
		{Name: "audit", Tables: []string{"logs"}},
		{Name: "ext", Tables: []string{"ext_setting"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeriveGroups mismatch\n got=%#v\nwant=%#v", got, want)
	}
}

func TestDeriveGroups_EmptyOrNilSchema(t *testing.T) {
	if got := DeriveGroups(nil); !reflect.DeepEqual(got, []Group{}) {
		t.Fatalf("nil schema should yield empty slice, got %#v", got)
	}
	s := &Schema{}
	if got := DeriveGroups(s); !reflect.DeepEqual(got, []Group{}) {
		t.Fatalf("empty schema should yield empty slice, got %#v", got)
	}
}

func TestDeriveGroups_GroupWithoutPrimaryMember(t *testing.T) {
	// Groups に登場するがどのテーブルも primary として宣言していない
	// ケース（secondary 指定だけのとき）。Tables は空のまま登場順を保つ。
	s := &Schema{
		Groups: []string{"audit"},
		Tables: []Table{
			{Name: "users", Groups: []string{"core", "audit"}},
		},
	}
	got := DeriveGroups(s)
	want := []Group{{Name: "audit", Tables: []string{}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeriveGroups mismatch\n got=%#v\nwant=%#v", got, want)
	}
}
