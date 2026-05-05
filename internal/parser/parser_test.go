// parser_test.go は internal/parser パッケージのユニットテスト。
//
// Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 3.3, 9.2, 9.5
package parser

import (
	"strings"
	"testing"

	"github.com/unok/erdm/internal/testutil/fixtures"
)

// TestParse_Fixtures は doc/sample/*.erdm をすべてパース成功させ、
// `@groups` 不在の既存ファイルがそのまま通る後方互換（要件 9.2）を確認する。
func TestParse_Fixtures(t *testing.T) {
	for _, name := range fixtures.NamesAll() {
		t.Run(name, func(t *testing.T) {
			data, err := fixtures.LoadFixture(name)
			if err != nil {
				t.Fatalf("load fixture %s: %v", name, err)
			}
			schema, perr := Parse(data)
			if perr != nil {
				t.Fatalf("Parse %s: %v", name, perr)
			}
			if schema == nil {
				t.Fatalf("Parse %s: returned nil schema with no error", name)
			}
			if len(schema.Tables) == 0 {
				t.Fatalf("Parse %s: schema has no tables", name)
			}
			// 既存サンプルは @groups を含まないので Schema.Groups は空のはず。
			if len(schema.Groups) != 0 {
				t.Fatalf("Parse %s: expected no groups for legacy sample, got %v", name, schema.Groups)
			}
			for _, tbl := range schema.Tables {
				if len(tbl.Groups) != 0 {
					t.Fatalf("Parse %s: table %s expected no groups, got %v", name, tbl.Name, tbl.Groups)
				}
			}
		})
	}
}

// TestParse_GroupsDeclWithLogicalName は論理名つきテーブルの @groups[...] が
// primary/secondary 順で取り込まれることを確認する（要件 2.1〜2.4, 2.7）。
func TestParse_GroupsDeclWithLogicalName(t *testing.T) {
	src := []byte("# Title: g\n" +
		"users / \"member\" @groups[\"core\", \"audit\"]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(schema.Tables))
	}
	tbl := schema.Tables[0]
	if tbl.Name != "users" {
		t.Errorf("Name=%q want users", tbl.Name)
	}
	if tbl.LogicalName != "member" {
		t.Errorf("LogicalName=%q want member", tbl.LogicalName)
	}
	wantGroups := []string{"core", "audit"}
	if len(tbl.Groups) != len(wantGroups) {
		t.Fatalf("Groups=%v want %v", tbl.Groups, wantGroups)
	}
	for i, g := range wantGroups {
		if tbl.Groups[i] != g {
			t.Errorf("Groups[%d]=%q want %q", i, tbl.Groups[i], g)
		}
	}
	primary, ok := tbl.PrimaryGroup()
	if !ok || primary != "core" {
		t.Errorf("PrimaryGroup=(%q,%v) want (core,true)", primary, ok)
	}
	if got := tbl.SecondaryGroups(); len(got) != 1 || got[0] != "audit" {
		t.Errorf("SecondaryGroups=%v want [audit]", got)
	}
	if len(schema.Groups) != 2 || schema.Groups[0] != "core" || schema.Groups[1] != "audit" {
		t.Errorf("Schema.Groups=%v want [core audit]", schema.Groups)
	}
}

// TestParse_GroupsDeclWithoutLogicalName は論理名なしテーブルでも @groups が
// 動作することを確認する。
func TestParse_GroupsDeclWithoutLogicalName(t *testing.T) {
	src := []byte("# Title: g\n" +
		"users @groups[\"core\"]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(schema.Tables))
	}
	tbl := schema.Tables[0]
	if tbl.LogicalName != "" {
		t.Errorf("LogicalName=%q want empty", tbl.LogicalName)
	}
	if len(tbl.Groups) != 1 || tbl.Groups[0] != "core" {
		t.Errorf("Groups=%v want [core]", tbl.Groups)
	}
}

// TestParse_SchemaGroupsAppearanceOrder は Schema.Groups が登場順で蓄積され、
// 重複は除外されることを確認する（要件 2.7）。
func TestParse_SchemaGroupsAppearanceOrder(t *testing.T) {
	src := []byte("# Title: g\n" +
		"a @groups[\"alpha\", \"beta\"]\n" +
		"    +id [bigserial][NN][U]\n" +
		"b @groups[\"beta\", \"gamma\"]\n" +
		"    +id [bigserial][NN][U]\n" +
		"c @groups[\"alpha\"]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	want := []string{"alpha", "beta", "gamma"}
	if len(schema.Groups) != len(want) {
		t.Fatalf("Schema.Groups=%v want %v", schema.Groups, want)
	}
	for i, g := range want {
		if schema.Groups[i] != g {
			t.Errorf("Schema.Groups[%d]=%q want %q", i, schema.Groups[i], g)
		}
	}
}

// TestParse_GroupsEmptyArrayRejected は @groups[]（空配列）が *ParseError で
// 失敗することを確認する（要件 2.5）。
func TestParse_GroupsEmptyArrayRejected(t *testing.T) {
	src := []byte("# Title: g\n" +
		"users @groups[]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr == nil {
		t.Fatalf("expected ParseError for empty @groups[], got schema=%v", schema)
	}
	if schema != nil {
		t.Errorf("expected nil schema on error, got %v", schema)
	}
	if perr.Line < 1 || perr.Column < 1 {
		t.Errorf("expected 1-based Line/Column, got Line=%d Column=%d", perr.Line, perr.Column)
	}
}

// TestParse_GroupsUnclosedQuoteRejected は引用符未閉じが *ParseError で
// 失敗することを確認する（要件 2.6）。
func TestParse_GroupsUnclosedQuoteRejected(t *testing.T) {
	src := []byte("# Title: g\n" +
		"users @groups[\"core\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr == nil {
		t.Fatalf("expected ParseError for unclosed quote, got schema=%v", schema)
	}
	if schema != nil {
		t.Errorf("expected nil schema on error, got %v", schema)
	}
}

// TestParse_GroupsCommaSeparatedInvalidRejected はカンマ区切りが不正
// （連続カンマ、空要素）な `@groups[...]` が *ParseError で失敗することを確認する
// （要件 2.6）。
func TestParse_GroupsCommaSeparatedInvalidRejected(t *testing.T) {
	src := []byte("# Title: g\n" +
		"users @groups[\"A\",,\"B\"]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr == nil {
		t.Fatalf("expected ParseError for invalid comma separator, got schema=%v", schema)
	}
	if schema != nil {
		t.Errorf("expected nil schema on error, got %v", schema)
	}
}

// TestParse_GroupsTopLevelBlockRejected はトップレベルブロック構文
// （`group "X" { ... }` 風の宣言）を `expression` ルールで受理しないことを
// 確認する（要件 2.8）。`group` 自体はテーブル名候補として一旦受理されるが、
// その後ろの `"X" { ... }` が文法に整合せず最終的にパース失敗する。
func TestParse_GroupsTopLevelBlockRejected(t *testing.T) {
	src := []byte("# Title: g\n" +
		"group \"X\" {\n" +
		"    +id [bigserial][NN][U]\n" +
		"}\n")
	schema, perr := Parse(src)
	if perr == nil {
		t.Fatalf("expected ParseError for top-level group block, got schema=%v", schema)
	}
	if schema != nil {
		t.Errorf("expected nil schema on error, got %v", schema)
	}
}

// TestParse_GroupsDuplicateOnSameTableRejected は同一テーブル宣言で
// `@groups[...]` を複数回宣言した場合に *ParseError で失敗することを確認する
// （要件 2.9）。文法側 `(groups_decl space*)?` が単一回しか受理しないため、
// 2 件目以降は宣言行末尾に置き去りとなり構文エラーとなる。
func TestParse_GroupsDuplicateOnSameTableRejected(t *testing.T) {
	src := []byte("# Title: g\n" +
		"users @groups[\"a\"] @groups[\"b\"]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr == nil {
		t.Fatalf("expected ParseError for duplicate @groups, got schema=%v", schema)
	}
	if schema != nil {
		t.Errorf("expected nil schema on error, got %v", schema)
	}
}

// TestParse_TitleCaptured は `# Title:` 行がスキーマタイトルに反映されることを確認する。
func TestParse_TitleCaptured(t *testing.T) {
	src := []byte("# Title: example\n" +
		"users\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	if schema.Title != "example" {
		t.Errorf("Title=%q want example", schema.Title)
	}
}

// TestParse_FKConverted は FK が model.FK に変換されることを確認する。
func TestParse_FKConverted(t *testing.T) {
	src := []byte("# Title: t\n" +
		"orders\n" +
		"    +id [bigserial][NN][U]\n" +
		"    user_id [bigint][NN] 0..*--1 users\n" +
		"users\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	if len(schema.Tables) != 2 {
		t.Fatalf("want 2 tables, got %d", len(schema.Tables))
	}
	orders := schema.Tables[0]
	if len(orders.Columns) != 2 {
		t.Fatalf("orders columns=%d want 2", len(orders.Columns))
	}
	userIDCol := orders.Columns[1]
	if !userIDCol.HasRelation() {
		t.Fatalf("user_id should be FK")
	}
	if userIDCol.FK.TargetTable != "users" {
		t.Errorf("FK.TargetTable=%q want users", userIDCol.FK.TargetTable)
	}
	if userIDCol.FK.CardinalitySource != "0..*" {
		t.Errorf("FK.CardinalitySource=%q want 0..*", userIDCol.FK.CardinalitySource)
	}
	if userIDCol.FK.CardinalityDestination != "1" {
		t.Errorf("FK.CardinalityDestination=%q want 1", userIDCol.FK.CardinalityDestination)
	}
	// FK でないカラムは FK==nil
	if orders.Columns[0].FK != nil {
		t.Errorf("orders.id should not have FK, got %+v", orders.Columns[0].FK)
	}
}

// TestParse_IndexConverted はインデックス情報が model.Index に変換され、
// Column.IndexRefs に逆参照が記録されることを確認する。
func TestParse_IndexConverted(t *testing.T) {
	src := []byte("# Title: t\n" +
		"users\n" +
		"    +id [bigserial][NN][U]\n" +
		"    name [text][NN]\n" +
		"    email [text][NN]\n" +
		"    index idx_name (name)\n" +
		"    index idx_email (email) unique\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	tbl := schema.Tables[0]
	if len(tbl.Indexes) != 2 {
		t.Fatalf("Indexes=%d want 2", len(tbl.Indexes))
	}
	if tbl.Indexes[0].Name != "idx_name" || tbl.Indexes[0].IsUnique {
		t.Errorf("idx_name unexpected: %+v", tbl.Indexes[0])
	}
	if tbl.Indexes[1].Name != "idx_email" || !tbl.Indexes[1].IsUnique {
		t.Errorf("idx_email unexpected: %+v", tbl.Indexes[1])
	}
	// IndexRefs の逆参照（カラム → インデックス添字）
	nameCol := tbl.Columns[1]
	if len(nameCol.IndexRefs) != 1 || nameCol.IndexRefs[0] != 0 {
		t.Errorf("name.IndexRefs=%v want [0]", nameCol.IndexRefs)
	}
	emailCol := tbl.Columns[2]
	if len(emailCol.IndexRefs) != 1 || emailCol.IndexRefs[0] != 1 {
		t.Errorf("email.IndexRefs=%v want [1]", emailCol.IndexRefs)
	}
}

// TestParse_PrimaryKeyAtNonFirstColumn は主キー（`+`）マーカーが先頭カラム
// 以外の位置に出現したときに、当該カラムが正しく PK として認識されることを
// 確認する。旧実装は登場順を 0/1 で記録していたためカラム添字とずれて
// `IsPrimaryKey` が誤判定する潜在バグがあった（AI-NEW-internal-parser-builder）。
func TestParse_PrimaryKeyAtNonFirstColumn(t *testing.T) {
	src := []byte("# Title: t\n" +
		"users\n" +
		"    name [text][NN]\n" +
		"    +id [bigserial][NN][U]\n")
	schema, perr := Parse(src)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(schema.Tables))
	}
	tbl := schema.Tables[0]
	if len(tbl.Columns) != 2 {
		t.Fatalf("Columns=%d want 2", len(tbl.Columns))
	}
	if tbl.Columns[0].Name != "name" {
		t.Errorf("Columns[0].Name=%q want name", tbl.Columns[0].Name)
	}
	if tbl.Columns[0].IsPrimaryKey {
		t.Errorf("Columns[0] (name) should not be PK")
	}
	if tbl.Columns[1].Name != "id" {
		t.Errorf("Columns[1].Name=%q want id", tbl.Columns[1].Name)
	}
	if !tbl.Columns[1].IsPrimaryKey {
		t.Errorf("Columns[1] (id) should be PK")
	}
}

// TestParseError_Format は ParseError.Error の形式を確認する。
func TestParseError_Format(t *testing.T) {
	e := &ParseError{Pos: 10, Line: 2, Column: 3, Message: "syntax error"}
	got := e.Error()
	if !strings.Contains(got, "line 2") || !strings.Contains(got, "column 3") || !strings.Contains(got, "syntax error") {
		t.Errorf("Error()=%q does not contain expected fields", got)
	}
}

// TestNewParseError_LineColumn は newParseError が 1-based Line/Column を
// 正しく算出することを確認する。
func TestNewParseError_LineColumn(t *testing.T) {
	buf := "abc\ndef\nghi"
	cases := []struct {
		pos        int
		wantLine   int
		wantColumn int
	}{
		{0, 1, 1},        // 'a'
		{2, 1, 3},        // 'c'
		{3, 1, 4},        // '\n' の位置（行末扱い）
		{4, 2, 1},        // 'd'
		{6, 2, 3},        // 'f'
		{8, 3, 1},        // 'g'
		{len(buf), 3, 4}, // EOF（'i' の次）
	}
	for _, tc := range cases {
		e := newParseError(tc.pos, buf, "x")
		if e.Line != tc.wantLine || e.Column != tc.wantColumn {
			t.Errorf("pos=%d Line=%d Column=%d, want Line=%d Column=%d",
				tc.pos, e.Line, e.Column, tc.wantLine, tc.wantColumn)
		}
	}
}
