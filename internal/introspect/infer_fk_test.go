package introspect

import (
	"reflect"
	"testing"

	"github.com/unok/erdm/internal/model"
)

// TestInferFKTarget は inferFKTarget の純粋ロジックを表駆動で固定する。
//
// inflection ライブラリ（github.com/jinzhu/inflection）に委ねている部分
// （`agency` → `agencies` / `media` → `media` / `person` → `people` 等の
// 不規則・不可算）も含めて、取込み時の振る舞いを契約として固める。
func TestInferFKTarget(t *testing.T) {
	t.Parallel()
	tableSet := func(names ...string) map[string]struct{} {
		m := make(map[string]struct{}, len(names))
		for _, n := range names {
			m[n] = struct{}{}
		}
		return m
	}
	cases := []struct {
		name      string
		col       string
		ownTable  string
		tables    map[string]struct{}
		wantOk    bool
		wantTable string
	}{
		// 規則変化: tenant_id -> tenants
		{name: "regular plural", col: "tenant_id", ownTable: "audit_logs", tables: tableSet("tenants", "audit_logs"), wantOk: true, wantTable: "tenants"},
		// y → ies: system_agency_id -> system_agencies
		{name: "y -> ies", col: "system_agency_id", ownTable: "halls", tables: tableSet("system_agencies", "halls"), wantOk: true, wantTable: "system_agencies"},
		// 不可算: system_media_id -> system_media（Plural が同形）
		{name: "uncountable media", col: "system_media_id", ownTable: "halls", tables: tableSet("system_media", "halls"), wantOk: true, wantTable: "system_media"},
		// data も不可算
		{name: "uncountable data", col: "data_id", ownTable: "users", tables: tableSet("data"), wantOk: true, wantTable: "data"},
		// 不規則: person → people
		{name: "irregular person -> people", col: "person_id", ownTable: "events", tables: tableSet("people"), wantOk: true, wantTable: "people"},
		// 単数形テーブル名のフォールバック（Plural ヒット無し → base 自身を試す）
		{name: "singular table fallback", col: "user_id", ownTable: "orders", tables: tableSet("user", "orders"), wantOk: true, wantTable: "user"},
		// 自テーブル参照は許容しない（明示 FK を要求する保守的方針）
		{name: "self-reference not inferred", col: "user_id", ownTable: "users", tables: tableSet("users"), wantOk: false},
		// 末尾が `_id` でない
		{name: "no _id suffix", col: "title", ownTable: "events", tables: tableSet("titles"), wantOk: false},
		// 列名が `id` 単独（PK 慣習）
		{name: "bare id is not a FK candidate", col: "id", ownTable: "events", tables: tableSet("events"), wantOk: false},
		// 候補テーブルが存在しない
		{name: "no matching table", col: "category_id", ownTable: "products", tables: tableSet("products"), wantOk: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := inferFKTarget(c.col, c.ownTable, c.tables)
			if ok != c.wantOk {
				t.Fatalf("ok=%v want %v (got=%q)", ok, c.wantOk, got)
			}
			if got != c.wantTable {
				t.Fatalf("table=%q want %q", got, c.wantTable)
			}
		})
	}
}

// TestInferNamingConventionFKs はスキーマ全体に対する適用結果を検証する。
//
//   - 既に明示 FK のある列は触らない
//   - 規約一致列に FK を補完する
//   - 自テーブル参照は補完しない
//   - カーディナリティは NOT NULL / UNIQUE 性に従う（decideFKCardinality）
func TestInferNamingConventionFKs(t *testing.T) {
	schema := &model.Schema{
		Title: "t",
		Tables: []model.Table{
			{
				Name: "tenants",
				Columns: []model.Column{
					{Name: "id", IsPrimaryKey: true, AllowNull: false, IsUnique: true, Type: "uuid"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name: "halls",
				Columns: []model.Column{
					{Name: "id", IsPrimaryKey: true, AllowNull: false, IsUnique: true, Type: "uuid"},
					{Name: "tenant_id", AllowNull: false, Type: "uuid"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name: "audit_logs",
				Columns: []model.Column{
					{Name: "id", IsPrimaryKey: true, AllowNull: false, IsUnique: true, Type: "uuid"},
					// 既に明示 FK あり: 推測で上書きされない
					{Name: "tenant_id", AllowNull: false, Type: "uuid", FK: &model.FK{
						TargetTable:            "explicit_tenants",
						CardinalitySource:      "1..*",
						CardinalityDestination: "1",
					}},
					// nullable: 0..*--1 になる
					{Name: "hall_id", AllowNull: true, Type: "uuid"},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name: "users",
				Columns: []model.Column{
					{Name: "id", IsPrimaryKey: true, AllowNull: false, IsUnique: true, Type: "uuid"},
					// 自テーブル参照候補（users 自身）→ 補完しない
					{Name: "user_id", AllowNull: false, Type: "uuid"},
				},
				PrimaryKeys: []int{0},
			},
		},
	}
	inferNamingConventionFKs(schema)

	// halls.tenant_id: 推測 FK → tenants
	if got := schema.Tables[1].Columns[1].FK; got == nil {
		t.Fatalf("halls.tenant_id should have inferred FK; got nil")
	} else {
		want := &model.FK{TargetTable: "tenants", CardinalitySource: "1..*", CardinalityDestination: "1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("halls.tenant_id FK=%+v want %+v", got, want)
		}
	}
	// audit_logs.tenant_id: 既存の明示 FK が温存される
	if got := schema.Tables[2].Columns[1].FK; got == nil || got.TargetTable != "explicit_tenants" {
		t.Errorf("audit_logs.tenant_id explicit FK was clobbered: %+v", got)
	}
	// audit_logs.hall_id (nullable): 0..*--1 に推測される
	if got := schema.Tables[2].Columns[2].FK; got == nil {
		t.Fatalf("audit_logs.hall_id should have inferred FK; got nil")
	} else {
		want := &model.FK{TargetTable: "halls", CardinalitySource: "0..*", CardinalityDestination: "1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("audit_logs.hall_id FK=%+v want %+v", got, want)
		}
	}
	// users.user_id: 自テーブル参照は推測しない
	if got := schema.Tables[3].Columns[1].FK; got != nil {
		t.Errorf("users.user_id self-reference should not be inferred; got %+v", got)
	}
}
