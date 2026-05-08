// schema_test.go は internal/model.Schema の不変条件と派生計算のテスト。
//
// Requirements: 3.2
package model

import (
	"errors"
	"strings"
	"testing"
)

func TestSchema_Validate_OK(t *testing.T) {
	s := &Schema{
		Title:  "ok",
		Groups: []string{"core"},
		Tables: []Table{
			{
				Name:        "users",
				Columns:     []Column{{Name: "id"}, {Name: "tenant_id"}},
				PrimaryKeys: []int{0},
				Groups:      []string{"core"},
			},
			{
				Name:    "logs",
				Columns: []Column{{Name: "id"}, {Name: "user_id", FK: &FK{TargetTable: "users"}}},
			},
		},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate must succeed: %v", err)
	}
}

func TestSchema_Validate_NilSchema(t *testing.T) {
	var s *Schema
	if err := s.Validate(); err == nil {
		t.Fatalf("nil schema must fail validation")
	}
}

func TestSchema_Validate_DuplicateTableName(t *testing.T) {
	s := &Schema{
		Tables: []Table{
			{Name: "users", Columns: []Column{{Name: "id"}}},
			{Name: "users", Columns: []Column{{Name: "id"}}},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatalf("duplicate table name must fail")
	}
	if !strings.Contains(err.Error(), "duplicate table name") {
		t.Fatalf("error should mention duplicate table name: %v", err)
	}
}

func TestSchema_Validate_EmptyTableName(t *testing.T) {
	s := &Schema{
		Tables: []Table{{Name: "", Columns: []Column{{Name: "id"}}}},
	}
	if err := s.Validate(); err == nil {
		t.Fatalf("empty table name must fail")
	}
}

func TestSchema_Validate_PrimaryKeyOutOfRange(t *testing.T) {
	s := &Schema{
		Tables: []Table{
			{Name: "users", Columns: []Column{{Name: "id"}}, PrimaryKeys: []int{5}},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatalf("PK out of range must fail")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("error should mention out of range: %v", err)
	}
}

func TestSchema_Validate_FKTargetMissing(t *testing.T) {
	s := &Schema{
		Tables: []Table{
			{
				Name:    "logs",
				Columns: []Column{{Name: "user_id", FK: &FK{TargetTable: "users"}}},
			},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatalf("FK target missing must fail")
	}
	if !strings.Contains(err.Error(), "users") {
		t.Fatalf("error should mention target table name: %v", err)
	}
}

func TestSchema_Validate_EmptyGroupName(t *testing.T) {
	s := &Schema{
		Groups: []string{"core", ""},
		Tables: []Table{{Name: "u", Columns: []Column{{Name: "id"}}, Groups: []string{"core", "  "}}},
	}
	err := s.Validate()
	if err == nil {
		t.Fatalf("empty group name must fail")
	}
}

func TestSchema_Validate_AggregatesMultipleErrors(t *testing.T) {
	s := &Schema{
		Tables: []Table{
			{Name: "", Columns: []Column{{Name: "id"}}, PrimaryKeys: []int{2}},
			{Name: "logs", Columns: []Column{{Name: "user_id", FK: &FK{TargetTable: "missing"}}}},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatalf("multiple violations must fail")
	}

	// errors.Join で集約されているため Unwrap[] で複数取り出せる必要がある。
	type unwrap interface{ Unwrap() []error }
	uw, ok := err.(unwrap)
	if !ok {
		t.Fatalf("Validate must return errors.Join result implementing Unwrap()[]")
	}
	if len(uw.Unwrap()) < 3 {
		t.Fatalf("expected at least 3 violations, got %d", len(uw.Unwrap()))
	}

	// 個別の違反は *ValidationError として取り出せる。
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate must return *ValidationError via errors.As")
	}
}

func TestSchema_DeriveGroups_Wrapper(t *testing.T) {
	s := &Schema{
		Groups: []string{"core"},
		Tables: []Table{{Name: "users", Groups: []string{"core"}}},
	}
	gs := s.DeriveGroups()
	if len(gs) != 1 || gs[0].Name != "core" || len(gs[0].Tables) != 1 || gs[0].Tables[0] != "users" {
		t.Fatalf("Schema.DeriveGroups returned unexpected: %#v", gs)
	}
}
