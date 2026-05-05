// roundtrip_test.go は internal/serializer の往復冪等性テスト。
//
// Requirements: 7.10
package serializer

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/unok/erdm/internal/model"
	"github.com/unok/erdm/internal/parser"
	"github.com/unok/erdm/internal/testutil/fixtures"
)

// TestRoundTrip_Fixtures は doc/sample/*.erdm 全件で
//
//	parse(input)            → schema0
//	serialize(schema0)      → text1
//	parse(text1)            → schema1
//	serialize(schema1)      → text2
//
// を実行し、要件 7.10 の不動点性（text1 == text2）と意味的同一性
// （reflect.DeepEqual(schema0, schema1)）を検証する。要件 9.2 の既存サンプル
// 後方互換テストも兼ねる。
func TestRoundTrip_Fixtures(t *testing.T) {
	for _, name := range fixtures.NamesAll() {
		t.Run(name, func(t *testing.T) {
			data, err := fixtures.LoadFixture(name)
			if err != nil {
				t.Fatalf("load fixture %s: %v", name, err)
			}
			schema0, perr := parser.Parse(data)
			if perr != nil {
				t.Fatalf("parse %s: %v", name, perr)
			}
			assertFixedPointAndSemanticIdentity(t, schema0)
		})
	}
}

// TestRoundTrip_GroupsPrimaryAndSecondary は `@groups` 入りスキーマの
// 往復で primary/secondary の登場順とテーブル所属が保持されることを確認する。
func TestRoundTrip_GroupsPrimaryAndSecondary(t *testing.T) {
	src := []byte("# Title: g\n" +
		"\n" +
		"users @groups[\"core\", \"audit\"]\n" +
		"    +id [bigserial][NN][U]\n" +
		"\n" +
		"orders @groups[\"core\"]\n" +
		"    +id [bigserial][NN][U]\n" +
		"    user_id [bigint][NN] 0..*--1 users\n")
	schema0, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	assertFixedPointAndSemanticIdentity(t, schema0)
	// 加えてこの入力では Schema.Groups の登場順 [core, audit] が保たれること
	if got := schema0.Groups; len(got) != 2 || got[0] != "core" || got[1] != "audit" {
		t.Errorf("Schema.Groups=%v want [core audit]", got)
	}
}

// TestRoundTrip_LogicalNameQuotedWithSpace は引用符で囲まれた論理名
// （空白を含む）が往復後も同形式で出力されることを確認する。
func TestRoundTrip_LogicalNameQuotedWithSpace(t *testing.T) {
	src := []byte("# Title: t\n" +
		"\n" +
		"users/\"site user master\"\n" +
		"    +id/\"member id\" [bigserial][NN][U]\n")
	schema0, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	assertFixedPointAndSemanticIdentity(t, schema0)
}

// TestRoundTrip_AllFlagsAndComments は属性 4 種・FK・コメントが往復後も
// 保持されることを確認する。
func TestRoundTrip_AllFlagsAndComments(t *testing.T) {
	src := []byte("# Title: t\n" +
		"\n" +
		"users\n" +
		"    +id [bigserial][NN][U]\n" +
		"    name [varchar(128)][NN][='']\n" +
		"    password [varchar(128)][='********']\n" +
		"     # sha1 でハッシュ化して登録\n" +
		"    updated [timestamp with time zone][NN][=now()][-erd]\n" +
		"    index i_users_name (name)\n")
	schema0, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	assertFixedPointAndSemanticIdentity(t, schema0)
}

// TestRoundTrip_CompoundPrimaryKey は複合主キーが往復後も保持されることを確認する。
// 旧パーサ仕様（`+`/`*` の登場順を 0/1 で記録）と整合させ、PrimaryKeys=[0,1] と
// Column.IsPrimaryKey の両方が保たれる。
func TestRoundTrip_CompoundPrimaryKey(t *testing.T) {
	src := []byte("# Title: t\n" +
		"\n" +
		"uk\n" +
		"    +a [int][NN]\n" +
		"    +b [int][NN]\n")
	schema0, perr := parser.Parse(src)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	assertFixedPointAndSemanticIdentity(t, schema0)
	if got := schema0.Tables[0].PrimaryKeys; len(got) != 2 {
		t.Errorf("PrimaryKeys=%v want length 2", got)
	}
}

// assertFixedPointAndSemanticIdentity は schema0 から往復シリアライズを行い、
// 不動点性（text1==text2）と意味的同一性（schema0≅schema1）を検証する。
func assertFixedPointAndSemanticIdentity(t *testing.T, schema0 *model.Schema) {
	t.Helper()
	text1, err := Serialize(schema0)
	if err != nil {
		t.Fatalf("Serialize#1: %v", err)
	}
	schema1, perr := parser.Parse(text1)
	if perr != nil {
		t.Fatalf("Parse(text1): %v\ntext1=\n%s", perr, string(text1))
	}
	text2, err := Serialize(schema1)
	if err != nil {
		t.Fatalf("Serialize#2: %v", err)
	}
	if !bytes.Equal(text1, text2) {
		t.Errorf("not fixed-point\ntext1=\n%s\ntext2=\n%s", string(text1), string(text2))
	}
	if !reflect.DeepEqual(schema0, schema1) {
		t.Errorf("schema not semantically identical\nschema0=%#v\nschema1=%#v", schema0, schema1)
	}
}
