package model

import "testing"

func TestIndex_ColumnNames(t *testing.T) {
	cases := []struct {
		name string
		idx  Index
		want string
	}{
		{"empty", Index{Columns: nil}, ""},
		{"single", Index{Columns: []string{"a"}}, "a"},
		{"multiple", Index{Columns: []string{"a", "b", "c"}}, "a, b, c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.idx.ColumnNames(); got != tc.want {
				t.Fatalf("ColumnNames()=%q want %q", got, tc.want)
			}
		})
	}
}
