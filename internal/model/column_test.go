package model

import "testing"

func TestColumn_HasDefault(t *testing.T) {
	cases := []struct {
		name string
		col  Column
		want bool
	}{
		{"empty", Column{Default: ""}, false},
		{"set", Column{Default: "0"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.col.HasDefault(); got != tc.want {
				t.Fatalf("HasDefault()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestColumn_HasComment(t *testing.T) {
	cases := []struct {
		name string
		col  Column
		want bool
	}{
		{"nil", Column{Comments: nil}, false},
		{"empty slice", Column{Comments: []string{}}, false},
		{"one", Column{Comments: []string{"hello"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.col.HasComment(); got != tc.want {
				t.Fatalf("HasComment()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestColumn_HasRelation(t *testing.T) {
	none := Column{FK: nil}
	if none.HasRelation() {
		t.Fatalf("nil FK should not have relation")
	}

	withFK := Column{FK: &FK{TargetTable: "users"}}
	if !withFK.HasRelation() {
		t.Fatalf("non-nil FK should have relation")
	}
}
