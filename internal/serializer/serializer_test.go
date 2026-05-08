// serializer_test.go は internal/serializer.Serialize のユニットテスト。
//
// Requirements: 7.6, 7.10
package serializer

import (
	"strings"
	"testing"

	"github.com/unok/erdm/internal/model"
)

// TestSerialize_Minimal はタイトルのみのスキーマが `# Title: <t>\n` で
// 書き出されることを確認する。
func TestSerialize_Minimal(t *testing.T) {
	s := &model.Schema{Title: "x"}
	got, err := Serialize(s)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := "# Title: x\n"
	if string(got) != want {
		t.Errorf("got=%q want=%q", string(got), want)
	}
}

// TestSerialize_SingleTableSinglePK は単一カラム・主キー 1 件の最小ケース。
func TestSerialize_SingleTableSinglePK(t *testing.T) {
	s := schemaWithUsersID()
	got, err := Serialize(s)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := "# Title: t\n" +
		"\n" +
		"users\n" +
		"    +id [bigserial][NN][U]\n"
	if string(got) != want {
		t.Errorf("got=\n%s\nwant=\n%s", string(got), want)
	}
}

// TestSerialize_LogicalNameQuoting は論理名が空白を含む場合に二重引用符で
// 囲み、含まない場合は無引用で出力されることを確認する（要件 9.2）。
func TestSerialize_LogicalNameQuoting(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name:        "users",
			LogicalName: "site user master",
			Columns: []model.Column{{
				Name: "id", Type: "bigserial",
				LogicalName:  "member id",
				AllowNull:    false,
				IsUnique:     true,
				IsPrimaryKey: true,
			}},
			PrimaryKeys: []int{0},
		}},
	}
	got, _ := Serialize(s)
	gotStr := string(got)
	if !strings.Contains(gotStr, `users/"site user master"`) {
		t.Errorf("table logical name not quoted in:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, `+id/"member id"`) {
		t.Errorf("column logical name not quoted in:\n%s", gotStr)
	}
}

// TestSerialize_LogicalNameNoQuoting は論理名が日本語など `[\t\r\n/ ]` を
// 含まない場合に無引用で出力されることを確認する。
func TestSerialize_LogicalNameNoQuoting(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name:        "users",
			LogicalName: "会員",
			Columns: []model.Column{{
				Name: "id", Type: "bigint", LogicalName: "会員ID",
				AllowNull: false, IsUnique: true, IsPrimaryKey: true,
			}},
			PrimaryKeys: []int{0},
		}},
	}
	got, _ := Serialize(s)
	gotStr := string(got)
	if !strings.Contains(gotStr, "users/会員\n") {
		t.Errorf("table logical name should not be quoted in:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "+id/会員ID ") {
		t.Errorf("column logical name should not be quoted in:\n%s", gotStr)
	}
}

// TestSerialize_GroupsDeclSingleAndMultiple は `@groups[...]` の出力規則
// （要素を二重引用符で囲み、カンマ + 半角スペース 1 個で連結）を確認する。
func TestSerialize_GroupsDeclSingleAndMultiple(t *testing.T) {
	cases := []struct {
		name    string
		groups  []string
		wantSub string
	}{
		{"single", []string{"core"}, ` @groups["core"]`},
		{"primary+secondary", []string{"core", "audit"}, ` @groups["core", "audit"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &model.Schema{
				Title:  "t",
				Groups: tc.groups,
				Tables: []model.Table{{
					Name:        "users",
					Groups:      tc.groups,
					Columns:     []model.Column{intPKColumn("id")},
					PrimaryKeys: []int{0},
				}},
			}
			got, _ := Serialize(s)
			if !strings.Contains(string(got), tc.wantSub) {
				t.Errorf("missing %q in:\n%s", tc.wantSub, string(got))
			}
		})
	}
}

// TestSerialize_FlagOrderFixed はカラム属性が `[NN] → [U] → [=default] → [-erd]`
// の固定順で出力されることを確認する。
func TestSerialize_FlagOrderFixed(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name: "users",
			Columns: []model.Column{{
				Name: "id", Type: "int",
				AllowNull: false, IsUnique: true, IsPrimaryKey: true,
				Default: "0", WithoutErd: true,
			}},
			PrimaryKeys: []int{0},
		}},
	}
	got, _ := Serialize(s)
	want := "+id [int][NN][U][=0][-erd]"
	if !strings.Contains(string(got), want) {
		t.Errorf("flag order mismatch in:\n%s", string(got))
	}
}

// TestSerialize_FK は FK のカーディナリティ書式を確認する。
func TestSerialize_FK(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{
			{
				Name: "orders",
				Columns: []model.Column{
					intPKColumn("id"),
					{
						Name: "user_id", Type: "bigint", AllowNull: false,
						FK: &model.FK{
							TargetTable:            "users",
							CardinalitySource:      "0..*",
							CardinalityDestination: "1",
						},
					},
				},
				PrimaryKeys: []int{0},
			},
			{
				Name:        "users",
				Columns:     []model.Column{intPKColumn("id")},
				PrimaryKeys: []int{0},
			},
		},
	}
	got, _ := Serialize(s)
	want := "    user_id [bigint][NN] 0..*--1 users\n"
	if !strings.Contains(string(got), want) {
		t.Errorf("FK format mismatch in:\n%s", string(got))
	}
}

// TestSerialize_Index はインデックス（unique / 通常）の書式を確認する。
func TestSerialize_Index(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name: "users",
			Columns: []model.Column{
				intPKColumn("id"),
				{Name: "email", Type: "text", AllowNull: false},
			},
			PrimaryKeys: []int{0},
			Indexes: []model.Index{
				{Name: "idx_email", Columns: []string{"email"}, IsUnique: false},
				{Name: "idx_email_u", Columns: []string{"email"}, IsUnique: true},
			},
		}},
	}
	got, _ := Serialize(s)
	gotStr := string(got)
	if !strings.Contains(gotStr, "    index idx_email (email)\n") {
		t.Errorf("normal index format mismatch in:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "    index idx_email_u (email) unique\n") {
		t.Errorf("unique index format mismatch in:\n%s", gotStr)
	}
}

// TestSerialize_CompoundPK は複合主キー時に全 PK カラムへ `+` が付与されることを確認する。
func TestSerialize_CompoundPK(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name: "uk",
			Columns: []model.Column{
				{Name: "a", Type: "int", AllowNull: false, IsPrimaryKey: true},
				{Name: "b", Type: "int", AllowNull: false, IsPrimaryKey: true},
			},
			PrimaryKeys: []int{0, 1},
		}},
	}
	got, _ := Serialize(s)
	gotStr := string(got)
	if !strings.Contains(gotStr, "    +a [int][NN]\n") {
		t.Errorf("first PK column missing `+` in:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "    +b [int][NN]\n") {
		t.Errorf("second PK column missing `+` in:\n%s", gotStr)
	}
}

// TestSerialize_Comments はカラムコメントが 5 スペース + `# ` 形式で
// 出力されることを確認する（複数件・順序保持）。
func TestSerialize_Comments(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name: "users",
			Columns: []model.Column{{
				Name: "password", Type: "varchar(128)", AllowNull: false,
				Default: "'********'",
				Comments: []string{
					"sha1 でハッシュ化して登録",
					"二行目",
				},
			}},
		}},
	}
	got, _ := Serialize(s)
	gotStr := string(got)
	if !strings.Contains(gotStr, "     # sha1 でハッシュ化して登録\n") {
		t.Errorf("first comment not formatted in:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "     # 二行目\n") {
		t.Errorf("second comment not formatted in:\n%s", gotStr)
	}
}

// TestSerialize_TableSeparation はテーブル間に空行 1 行が挿入されることを確認する。
func TestSerialize_TableSeparation(t *testing.T) {
	s := &model.Schema{
		Title: "t",
		Tables: []model.Table{
			{Name: "a", Columns: []model.Column{intPKColumn("id")}, PrimaryKeys: []int{0}},
			{Name: "b", Columns: []model.Column{intPKColumn("id")}, PrimaryKeys: []int{0}},
		},
	}
	got, _ := Serialize(s)
	want := "# Title: t\n" +
		"\n" +
		"a\n" +
		"    +id [bigserial][NN][U]\n" +
		"\n" +
		"b\n" +
		"    +id [bigserial][NN][U]\n"
	if string(got) != want {
		t.Errorf("got=\n%s\nwant=\n%s", string(got), want)
	}
}

// schemaWithUsersID は最小のテストフィクスチャ（1 テーブル + 1 PK カラム）を返す。
func schemaWithUsersID() *model.Schema {
	return &model.Schema{
		Title: "t",
		Tables: []model.Table{{
			Name:        "users",
			Columns:     []model.Column{intPKColumn("id")},
			PrimaryKeys: []int{0},
		}},
	}
}

// intPKColumn は `+<name> [bigserial][NN][U]` 相当の主キーカラムを返す。
func intPKColumn(name string) model.Column {
	return model.Column{
		Name:         name,
		Type:         "bigserial",
		AllowNull:    false,
		IsUnique:     true,
		IsPrimaryKey: true,
	}
}
