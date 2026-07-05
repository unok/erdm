package introspect

import (
	"reflect"
	"strings"
	"testing"
)

// newSQLiteIntrospector は接続済み *sql.DB を素通しで保持する。スキーマ指定は
// 受け付けないため、コンストラクタ呼び出しが安全に nil DB でも完了することを
// 確認する。SQLite はファイル単位接続のためスキーマ概念を持たない（要件 2.3）。
func TestNewSQLiteIntrospector_StoresDB(t *testing.T) {
	t.Parallel()
	got := newSQLiteIntrospector(nil)
	if got == nil {
		t.Fatalf("newSQLiteIntrospector returned nil")
	}
	if got.db != nil {
		t.Fatalf("db = %v, want nil", got.db)
	}
}

// normalizeSQLiteAutoIncrement は単一カラム PK で型が空または INTEGER（大小文字
// 無視）のときに自動 ROWID 連番列とみなし、Default を空にクリアし、空型は
// INTEGER に補正する（要件 4.7）。それ以外は入力をそのまま返す。
func TestNormalizeSQLiteAutoIncrement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		typeIn      string
		defaultIn   string
		isPK        bool
		isSinglePK  bool
		wantType    string
		wantDefault string
	}{
		{
			name:        "empty type with single PK becomes INTEGER and clears default",
			typeIn:      "",
			defaultIn:   "0",
			isPK:        true,
			isSinglePK:  true,
			wantType:    "INTEGER",
			wantDefault: "",
		},
		{
			name:        "INTEGER single PK clears default",
			typeIn:      "INTEGER",
			defaultIn:   "1",
			isPK:        true,
			isSinglePK:  true,
			wantType:    "INTEGER",
			wantDefault: "",
		},
		{
			name:        "lower-case integer single PK preserves casing and clears default",
			typeIn:      "integer",
			defaultIn:   "2",
			isPK:        true,
			isSinglePK:  true,
			wantType:    "integer",
			wantDefault: "",
		},
		{
			name:        "TEXT single PK preserves type and default",
			typeIn:      "TEXT",
			defaultIn:   "hello",
			isPK:        true,
			isSinglePK:  true,
			wantType:    "TEXT",
			wantDefault: "hello",
		},
		{
			name:        "INTEGER composite PK preserves type and default",
			typeIn:      "INTEGER",
			defaultIn:   "0",
			isPK:        true,
			isSinglePK:  false,
			wantType:    "INTEGER",
			wantDefault: "0",
		},
		{
			name:        "INTEGER non-PK preserves type and default",
			typeIn:      "INTEGER",
			defaultIn:   "0",
			isPK:        false,
			isSinglePK:  true,
			wantType:    "INTEGER",
			wantDefault: "0",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotDef := normalizeSQLiteAutoIncrement(c.typeIn, c.defaultIn, c.isPK, c.isSinglePK)
			if gotType != c.wantType || gotDef != c.wantDefault {
				t.Fatalf("normalizeSQLiteAutoIncrement(%q,%q,%v,%v) = (%q,%q), want (%q,%q)",
					c.typeIn, c.defaultIn, c.isPK, c.isSinglePK,
					gotType, gotDef, c.wantType, c.wantDefault)
			}
		})
	}
}

// derivePrimaryKey は pragma_table_info.pk の値（1..N）で並べた構成カラム順序を
// 返す純粋関数（要件 5.2）。pk == 0 のカラムは除外。
func TestDerivePrimaryKey(t *testing.T) {
	t.Parallel()
	cols := []rawColumn{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}
	cases := []struct {
		name   string
		pkSeqs []int
		want   []string
	}{
		{name: "no pk", pkSeqs: []int{0, 0, 0, 0}, want: []string{}},
		{name: "single pk on first column", pkSeqs: []int{1, 0, 0, 0}, want: []string{"a"}},
		{name: "single pk on last column", pkSeqs: []int{0, 0, 0, 1}, want: []string{"d"}},
		{name: "composite pk preserves declared order", pkSeqs: []int{2, 0, 1, 0}, want: []string{"c", "a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := derivePrimaryKey(cols, c.pkSeqs)
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("derivePrimaryKey(%v) = %v, want %v", c.pkSeqs, got, c.want)
				}
			}
		})
	}
}

// applyAutoIncrementToColumns は単一カラム PK のみで自動 ROWID 連番抑止を適用し、
// 複合 PK や非 PK 列には適用しないことを確認する。
func TestApplyAutoIncrementToColumns(t *testing.T) {
	t.Parallel()
	cols := []rawColumn{
		{Name: "id", Type: "INTEGER", Default: "rowid_default"},
		{Name: "name", Type: "TEXT", Default: "anon"},
	}
	applyAutoIncrementToColumns(cols, []int{1, 0}, true)
	if cols[0].Default != "" {
		t.Errorf("single PK INTEGER default not cleared: got %q", cols[0].Default)
	}
	if cols[1].Default != "anon" {
		t.Errorf("non-PK default mutated: got %q", cols[1].Default)
	}

	// composite PK: not applied
	cols = []rawColumn{
		{Name: "tenant_id", Type: "INTEGER", Default: "0"},
		{Name: "user_id", Type: "INTEGER", Default: "0"},
	}
	applyAutoIncrementToColumns(cols, []int{1, 2}, false)
	if cols[0].Default != "0" || cols[1].Default != "0" {
		t.Errorf("composite PK defaults mutated: got %v", cols)
	}
}

// buildSQLiteRawTable はカラムコメント反映・単一カラム UNIQUE 補完・FK
// SourceUnique 補完までを 1 関数で実施し、組み立て後の rawTable を返す。
func TestBuildSQLiteRawTable_AppliesCommentsAndUniqueFlags(t *testing.T) {
	t.Parallel()
	cols := []rawColumn{
		{Name: "id", Type: "INTEGER"},
		{Name: "email", Type: "TEXT"},
		{Name: "name", Type: "TEXT"},
	}
	comments := map[string]string{
		"id":    "識別子",
		"email": "メールアドレス",
	}
	uniqueCols := []string{"name"}
	pk := []string{"id"}
	fks := []rawForeignKey{
		{SourceColumns: []string{"email"}, TargetTable: "external"},
	}
	indexes := []rawIndex{
		{Name: "ux_users_email", Columns: []string{"email"}, IsUnique: true},
	}
	got := buildSQLiteRawTable("users", cols, comments, uniqueCols, pk, fks, indexes)
	if got.Name != "users" || got.Comment != "" {
		t.Fatalf("name/comment = %q/%q, want users/\"\"", got.Name, got.Comment)
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
		t.Errorf("expected name column to be marked unique via UNIQUE-constraint origin")
	}
	if !got.ForeignKeys[0].SourceUnique {
		t.Errorf("expected FK on email to inherit SourceUnique=true from email IsUnique")
	}
	if !reflect.DeepEqual(got.PrimaryKey, []string{"id"}) {
		t.Errorf("primary key = %v, want [id]", got.PrimaryKey)
	}
}

// columnNames は rawColumn 列から物理名スライスを抽出する純粋ヘルパ。
// extractSQLiteColumnComments への入力組み立てで使うため、空入力と単一・複数を
// 抑える表駆動で確認する。
func TestColumnNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []rawColumn
		want []string
	}{
		{name: "empty", in: nil, want: []string{}},
		{name: "single", in: []rawColumn{{Name: "id"}}, want: []string{"id"}},
		{name: "multiple preserves order", in: []rawColumn{{Name: "a"}, {Name: "b"}, {Name: "c"}}, want: []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := columnNames(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d", len(got), len(c.want))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got = %v, want %v", got, c.want)
				}
			}
		})
	}
}

// SQL／PRAGMA 定数のスモークテスト。SQL の細かな構文ではなく、`sqlite_master` /
// `pragma_*` の主要キーワードがクエリから抜け落ちていないかをガードする。
// 本格的な振る舞い検証はタスク 10.3 の統合テストの責務。
func TestSQLiteSQLConstantsCarryRequiredKeywords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "tables",
			sql:  sqlSelectSQLiteTables,
			want: []string{"sqlite_master", "type = 'table'", "sqlite_%"},
		},
		{
			name: "columns",
			sql:  sqlSelectSQLiteColumns,
			want: []string{"pragma_table_info", "cid", `"notnull"`, "dflt_value", "pk"},
		},
		{
			name: "foreign keys",
			sql:  sqlSelectSQLiteForeignKeys,
			want: []string{"pragma_foreign_key_list", `"table"`, `"from"`, "ORDER BY id, seq"},
		},
		{
			name: "index list",
			sql:  sqlSelectSQLiteIndexList,
			want: []string{"pragma_index_list", `"unique"`, "origin"},
		},
		{
			name: "index info",
			sql:  sqlSelectSQLiteIndexInfo,
			want: []string{"pragma_index_info", "seqno", "ORDER BY seqno"},
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
