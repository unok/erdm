package parser

import (
	"encoding/json"
	"strings"
	"testing"
)

// Go の encoding/json は nil スライスを null と書き出す。空の Groups / Comments などで
// SPA が .join を呼ぶと落ちるため、パーサ出力では空でも非 nil スライスに正規化する。
func TestMarshalSchemaNoNullSlicesForEmptyTableGroupsAndComments(t *testing.T) {
	src := []byte("# Title: t\n" +
		"users / \"Users\"\n" +
		"    +id [bigserial][NN][U]\n")
	s, perr := Parse(src)
	if perr != nil {
		t.Fatal(perr)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if strings.Contains(js, `"Groups":null`) {
		t.Fatalf("expected no null Groups slice in JSON: %s", js)
	}
	if strings.Contains(js, `"Comments":null`) {
		t.Fatalf("expected no null Comments slice in JSON: %s", js)
	}
	if strings.Contains(js, `"PrimaryKeys":null`) {
		t.Fatalf("expected no null PrimaryKeys slice in JSON: %s", js)
	}
	if strings.Contains(js, `"IndexRefs":null`) {
		t.Fatalf("expected no null IndexRefs slice in JSON: %s", js)
	}
}
