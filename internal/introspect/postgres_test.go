package introspect

import (
	"reflect"
	"strings"
	"testing"
)

// newPostgresIntrospector はスキーマ未指定時に既定 "public" を採用する
// （要件 3.4）。スケルトン段階でも観測可能な不変条件のため、ここで固定する。
func TestNewPostgresIntrospector_DefaultsToPublicWhenSchemaEmpty(t *testing.T) {
	t.Parallel()
	got := newPostgresIntrospector(nil, "")
	if got.schema != "public" {
		t.Fatalf("default schema = %q, want %q", got.schema, "public")
	}
}

// 明示指定された schema は素通しで採用される（要件 3.3）。
func TestNewPostgresIntrospector_HonorsExplicitSchema(t *testing.T) {
	t.Parallel()
	got := newPostgresIntrospector(nil, "analytics")
	if got.schema != "analytics" {
		t.Fatalf("schema = %q, want %q", got.schema, "analytics")
	}
}

// normalizePGSerial は `nextval(...)` + integer 系の組合わせのみを
// `smallserial`／`serial`／`bigserial` に正規化し、Default を空文字列にクリアする
// （要件 4.7）。それ以外の組合わせは入力をそのまま返す。
func TestNormalizePGSerial(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		dataType    string
		columnDef   string
		wantType    string
		wantDefault string
	}{
		{
			name:        "smallint with nextval becomes smallserial",
			dataType:    "smallint",
			columnDef:   "nextval('t_id_seq'::regclass)",
			wantType:    "smallserial",
			wantDefault: "",
		},
		{
			name:        "integer with nextval becomes serial",
			dataType:    "integer",
			columnDef:   "nextval('t_id_seq'::regclass)",
			wantType:    "serial",
			wantDefault: "",
		},
		{
			name:        "bigint with nextval becomes bigserial",
			dataType:    "bigint",
			columnDef:   "nextval('t_id_seq'::regclass)",
			wantType:    "bigserial",
			wantDefault: "",
		},
		{
			name:        "text with nextval is not a serial",
			dataType:    "text",
			columnDef:   "nextval('t_id_seq'::regclass)",
			wantType:    "text",
			wantDefault: "nextval('t_id_seq'::regclass)",
		},
		{
			name:        "integer with literal default is preserved",
			dataType:    "integer",
			columnDef:   "0",
			wantType:    "integer",
			wantDefault: "0",
		},
		{
			name:        "integer without default is preserved",
			dataType:    "integer",
			columnDef:   "",
			wantType:    "integer",
			wantDefault: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotDef := normalizePGSerial(c.dataType, c.columnDef)
			if gotType != c.wantType || gotDef != c.wantDefault {
				t.Fatalf("normalizePGSerial(%q,%q) = (%q,%q), want (%q,%q)",
					c.dataType, c.columnDef, gotType, gotDef, c.wantType, c.wantDefault)
			}
		})
	}
}

// resolvePGType は USER-DEFINED 型のときだけ udt_name に切り替える
// （他の型は data_type をそのまま採用する）。
func TestResolvePGType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		dataType string
		udtName  string
		want     string
	}{
		{name: "user defined falls back to udt", dataType: "USER-DEFINED", udtName: "mood", want: "mood"},
		{name: "integer is preserved", dataType: "integer", udtName: "int4", want: "integer"},
		{name: "text is preserved", dataType: "text", udtName: "text", want: "text"},
		{name: "user defined with empty udt is preserved", dataType: "USER-DEFINED", udtName: "", want: "USER-DEFINED"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := resolvePGType(c.dataType, c.udtName); got != c.want {
				t.Fatalf("resolvePGType(%q,%q) = %q, want %q", c.dataType, c.udtName, got, c.want)
			}
		})
	}
}

// applySingleColumnUnique は補助インデックス由来の単一カラム UNIQUE を
// rawColumn.IsUnique に反映する。複合 UNIQUE および非 UNIQUE は反映しない
// （要件 4.4）。
func TestApplySingleColumnUnique(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   rawTable
		wantSet map[string]bool
	}{
		{
			name: "single column unique index marks column",
			input: rawTable{
				Columns: []rawColumn{{Name: "email"}, {Name: "name"}},
				Indexes: []rawIndex{{Name: "ux_users_email", Columns: []string{"email"}, IsUnique: true}},
			},
			wantSet: map[string]bool{"email": true, "name": false},
		},
		{
			name: "composite unique index does not mark columns",
			input: rawTable{
				Columns: []rawColumn{{Name: "tenant_id"}, {Name: "email"}},
				Indexes: []rawIndex{{Name: "ux_users_tenant_email", Columns: []string{"tenant_id", "email"}, IsUnique: true}},
			},
			wantSet: map[string]bool{"tenant_id": false, "email": false},
		},
		{
			name: "non unique index does not mark column",
			input: rawTable{
				Columns: []rawColumn{{Name: "name"}},
				Indexes: []rawIndex{{Name: "ix_users_name", Columns: []string{"name"}, IsUnique: false}},
			},
			wantSet: map[string]bool{"name": false},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := c.input
			applySingleColumnUnique(&got)
			actual := map[string]bool{}
			for _, col := range got.Columns {
				actual[col.Name] = col.IsUnique
			}
			if !reflect.DeepEqual(actual, c.wantSet) {
				t.Fatalf("IsUnique map = %v, want %v", actual, c.wantSet)
			}
		})
	}
}

// applyFKSourceUnique は単一カラム FK の SourceUnique を rawColumn.IsUnique
// から導出する。複合 FK には反映しない（要件 6.3 / 6.4）。
func TestApplyFKSourceUnique(t *testing.T) {
	t.Parallel()
	tab := rawTable{
		Columns: []rawColumn{
			{Name: "id", IsUnique: true},
			{Name: "tenant_id"},
			{Name: "user_id"},
		},
		ForeignKeys: []rawForeignKey{
			{SourceColumns: []string{"id"}, TargetTable: "external_a"},
			{SourceColumns: []string{"user_id"}, TargetTable: "users"},
			{SourceColumns: []string{"tenant_id", "user_id"}, TargetTable: "tenant_users"},
		},
	}
	applyFKSourceUnique(&tab)
	wants := []bool{true, false, false}
	for i, w := range wants {
		if tab.ForeignKeys[i].SourceUnique != w {
			t.Errorf("FK[%d].SourceUnique = %v, want %v", i, tab.ForeignKeys[i].SourceUnique, w)
		}
	}
}

// markColumnUnique は最初に一致したカラムにだけ IsUnique=true を立て、
// 同名カラムが無い場合は何もしない（防御的に panic しない）。
func TestMarkColumnUnique(t *testing.T) {
	t.Parallel()
	cols := []rawColumn{{Name: "a"}, {Name: "b"}}
	markColumnUnique(cols, "missing")
	if cols[0].IsUnique || cols[1].IsUnique {
		t.Fatalf("missing column unexpectedly mutated state: %#v", cols)
	}
	markColumnUnique(cols, "b")
	if cols[0].IsUnique || !cols[1].IsUnique {
		t.Fatalf("expected only b to be marked unique, got %#v", cols)
	}
}

// buildPGRawTable はカラムコメント付与・単一カラム UNIQUE 補完・FK SourceUnique
// 補完までを 1 関数で実施し、組み立て後の rawTable を返す。
func TestBuildPGRawTable_AppliesCommentsAndUniqueFlags(t *testing.T) {
	t.Parallel()
	cols := []rawColumn{
		{Name: "id"},
		{Name: "email"},
		{Name: "name"},
	}
	colComments := map[tableColumnKey]string{
		{Table: "users", Column: "id"}:    "識別子",
		{Table: "users", Column: "email"}: "メールアドレス",
	}
	pk := []string{"id"}
	fks := []rawForeignKey{
		{SourceColumns: []string{"email"}, TargetTable: "external"},
	}
	indexes := []rawIndex{
		{Name: "ux_users_email", Columns: []string{"email"}, IsUnique: true},
	}
	got := buildPGRawTable("users", "ユーザー", cols, colComments, []string{"name"}, pk, fks, indexes)
	if got.Name != "users" || got.Comment != "ユーザー" {
		t.Fatalf("name/comment = %q/%q, want users/ユーザー", got.Name, got.Comment)
	}
	wantComments := []string{"識別子", "メールアドレス", ""}
	for i, w := range wantComments {
		if got.Columns[i].Comment != w {
			t.Errorf("column[%d].Comment = %q, want %q", i, got.Columns[i].Comment, w)
		}
	}
	if !got.Columns[1].IsUnique {
		t.Errorf("expected email column to be marked unique via index")
	}
	if !got.Columns[2].IsUnique {
		t.Errorf("expected name column to be marked unique via single-column UNIQUE constraint")
	}
	if !got.ForeignKeys[0].SourceUnique {
		t.Errorf("expected FK on email to inherit SourceUnique=true from email IsUnique")
	}
	if !reflect.DeepEqual(got.PrimaryKey, []string{"id"}) {
		t.Errorf("primary key = %v, want [id]", got.PrimaryKey)
	}
}

// SQL 定数のスモークテスト。SQL の細かな構文ではなく、`information_schema` /
// `pg_catalog` の主要キーワードがクエリから抜け落ちていないかをガードする。
// 本格的な振る舞い検証はタスク 10.1 の統合テストの責務。
func TestPGSQLConstantsCarryRequiredKeywords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "tables",
			sql:  sqlSelectPGTables,
			want: []string{"information_schema.tables", "BASE TABLE", "table_schema = $1"},
		},
		{
			name: "table comments",
			sql:  sqlSelectPGTableComments,
			want: []string{"pg_class", "pg_description", "objsubid = 0", "pg_namespace"},
		},
		{
			name: "columns",
			sql:  sqlSelectPGColumns,
			want: []string{"information_schema.columns", "ordinal_position", "udt_name", "is_nullable"},
		},
		{
			name: "column comments",
			sql:  sqlSelectPGColumnComments,
			want: []string{"pg_attribute", "pg_description", "attisdropped"},
		},
		{
			name: "primary keys",
			sql:  sqlSelectPGPrimaryKeys,
			want: []string{"PRIMARY KEY", "key_column_usage", "table_constraints"},
		},
		{
			name: "unique constraints",
			sql:  sqlSelectPGUniqueConstraints,
			want: []string{"'UNIQUE'", "key_column_usage", "constraint_name"},
		},
		{
			name: "foreign keys",
			sql:  sqlSelectPGForeignKeys,
			want: []string{"pg_constraint", "contype = 'f'", "unnest(con.conkey)", "WITH ORDINALITY"},
		},
		{
			name: "indexes",
			sql:  sqlSelectPGIndexes,
			want: []string{"pg_index", "indisunique", "indisprimary = false", "NOT EXISTS"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, kw := range c.want {
				if !strings.Contains(c.sql, kw) {
					t.Errorf("%s SQL missing keyword %q", c.name, kw)
				}
			}
		})
	}
}
