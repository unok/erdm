package introspect

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

// nullInt は sql.NullInt64 を簡潔に作るヘルパ。テストの可読性向上目的。
func nullInt(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

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

// resolvePGType は data_type が `USER-DEFINED` / `ARRAY` のときに udt_name を
// 経由した正規化を行い、それ以外は data_type をそのまま採用する。配列要素は
// pgArrayElementDisplay で `data_type` 寄りの表示名に揃える。
func TestResolvePGType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		dataType string
		udtName  string
		want     string
	}{
		// USER-DEFINED
		{name: "user defined falls back to udt", dataType: "USER-DEFINED", udtName: "mood", want: "mood"},
		{name: "user defined with empty udt is preserved", dataType: "USER-DEFINED", udtName: "", want: "USER-DEFINED"},
		// 非配列・非ユーザ定義: 短縮対象外は data_type そのまま
		{name: "integer is preserved", dataType: "integer", udtName: "int4", want: "integer"},
		{name: "text is preserved", dataType: "text", udtName: "text", want: "text"},
		// 非配列・短縮対象（character varying / timestamp / time）
		{name: "character varying -> varchar", dataType: "character varying", udtName: "varchar", want: "varchar"},
		{name: "timestamp without time zone -> timestamp", dataType: "timestamp without time zone", udtName: "timestamp", want: "timestamp"},
		{name: "timestamp with time zone -> timestamptz", dataType: "timestamp with time zone", udtName: "timestamptz", want: "timestamptz"},
		{name: "time without time zone -> time", dataType: "time without time zone", udtName: "time", want: "time"},
		{name: "time with time zone -> timetz", dataType: "time with time zone", udtName: "timetz", want: "timetz"},
		// ARRAY: 主要ビルトイン型
		{name: "array of int4 -> integer[]", dataType: "ARRAY", udtName: "_int4", want: "integer[]"},
		{name: "array of int8 -> bigint[]", dataType: "ARRAY", udtName: "_int8", want: "bigint[]"},
		{name: "array of int2 -> smallint[]", dataType: "ARRAY", udtName: "_int2", want: "smallint[]"},
		{name: "array of float4 -> real[]", dataType: "ARRAY", udtName: "_float4", want: "real[]"},
		{name: "array of float8 -> double precision[]", dataType: "ARRAY", udtName: "_float8", want: "double precision[]"},
		{name: "array of numeric -> numeric[]", dataType: "ARRAY", udtName: "_numeric", want: "numeric[]"},
		{name: "array of bool -> boolean[]", dataType: "ARRAY", udtName: "_bool", want: "boolean[]"},
		{name: "array of varchar -> varchar[]", dataType: "ARRAY", udtName: "_varchar", want: "varchar[]"},
		{name: "array of bpchar -> character[]", dataType: "ARRAY", udtName: "_bpchar", want: "character[]"},
		{name: "array of text -> text[]", dataType: "ARRAY", udtName: "_text", want: "text[]"},
		{name: "array of bytea -> bytea[]", dataType: "ARRAY", udtName: "_bytea", want: "bytea[]"},
		{name: "array of date -> date[]", dataType: "ARRAY", udtName: "_date", want: "date[]"},
		{name: "array of time -> time[]", dataType: "ARRAY", udtName: "_time", want: "time[]"},
		{name: "array of timetz -> timetz[]", dataType: "ARRAY", udtName: "_timetz", want: "timetz[]"},
		{name: "array of timestamp -> timestamp[]", dataType: "ARRAY", udtName: "_timestamp", want: "timestamp[]"},
		{name: "array of timestamptz -> timestamptz[]", dataType: "ARRAY", udtName: "_timestamptz", want: "timestamptz[]"},
		{name: "array of interval -> interval[]", dataType: "ARRAY", udtName: "_interval", want: "interval[]"},
		{name: "array of uuid -> uuid[]", dataType: "ARRAY", udtName: "_uuid", want: "uuid[]"},
		{name: "array of json -> json[]", dataType: "ARRAY", udtName: "_json", want: "json[]"},
		{name: "array of jsonb -> jsonb[]", dataType: "ARRAY", udtName: "_jsonb", want: "jsonb[]"},
		{name: "array of inet -> inet[]", dataType: "ARRAY", udtName: "_inet", want: "inet[]"},
		{name: "array of cidr -> cidr[]", dataType: "ARRAY", udtName: "_cidr", want: "cidr[]"},
		{name: "array of macaddr -> macaddr[]", dataType: "ARRAY", udtName: "_macaddr", want: "macaddr[]"},
		{name: "array of macaddr8 -> macaddr8[]", dataType: "ARRAY", udtName: "_macaddr8", want: "macaddr8[]"},
		{name: "array of bit -> bit[]", dataType: "ARRAY", udtName: "_bit", want: "bit[]"},
		{name: "array of varbit -> bit varying[]", dataType: "ARRAY", udtName: "_varbit", want: "bit varying[]"},
		{name: "array of money -> money[]", dataType: "ARRAY", udtName: "_money", want: "money[]"},
		{name: "array of xml -> xml[]", dataType: "ARRAY", udtName: "_xml", want: "xml[]"},
		{name: "array of tsvector -> tsvector[]", dataType: "ARRAY", udtName: "_tsvector", want: "tsvector[]"},
		{name: "array of tsquery -> tsquery[]", dataType: "ARRAY", udtName: "_tsquery", want: "tsquery[]"},
		// ARRAY of USER-DEFINED enum: udt_name=_<enumname>。マップに無いので
		// `_` だけ剥がして `<enumname>[]` を出す。
		{name: "array of user defined enum -> mood[]", dataType: "ARRAY", udtName: "_mood", want: "mood[]"},
		// ARRAY: udt_name 空欄は ARRAY のまま温存（情報損失を可視化する保守的経路）
		{name: "array with empty udt is preserved", dataType: "ARRAY", udtName: "", want: "ARRAY"},
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

// TestApplyPGTypeModifier は型表記に対する精度／スケール／長さ修飾子の付与を
// 表駆動で固定する（`varchar(N)` / `numeric(p,s)` / `timestamp(N)` 等）。
// 配列型（末尾 `[]`）は基底型に修飾子を付けて `[]` を再付与する。
func TestApplyPGTypeModifier(t *testing.T) {
	t.Parallel()
	none := sql.NullInt64{}
	cases := []struct {
		name    string
		typ     string
		charLen sql.NullInt64
		numP    sql.NullInt64
		numS    sql.NullInt64
		dtP     sql.NullInt64
		want    string
	}{
		// varchar / character: 長さがあれば常に付与
		{name: "varchar(128)", typ: "varchar", charLen: nullInt(128), want: "varchar(128)"},
		{name: "varchar without length stays varchar", typ: "varchar", want: "varchar"},
		{name: "character(8)", typ: "character", charLen: nullInt(8), want: "character(8)"},
		// numeric / decimal: precision があれば常に付与（scale は NULL を 0 扱い）
		{name: "numeric(10,2)", typ: "numeric", numP: nullInt(10), numS: nullInt(2), want: "numeric(10,2)"},
		{name: "numeric(10) -> numeric(10,0)", typ: "numeric", numP: nullInt(10), numS: nullInt(0), want: "numeric(10,0)"},
		{name: "numeric(10) with NULL scale", typ: "numeric", numP: nullInt(10), want: "numeric(10,0)"},
		{name: "numeric without precision stays numeric", typ: "numeric", want: "numeric"},
		{name: "decimal(20,5)", typ: "decimal", numP: nullInt(20), numS: nullInt(5), want: "decimal(20,5)"},
		// timestamp 系: 既定 6 は省略、それ以外は付与
		{name: "timestamp default precision is omitted", typ: "timestamp", dtP: nullInt(6), want: "timestamp"},
		{name: "timestamp(3)", typ: "timestamp", dtP: nullInt(3), want: "timestamp(3)"},
		{name: "timestamptz(0)", typ: "timestamptz", dtP: nullInt(0), want: "timestamptz(0)"},
		{name: "time(2)", typ: "time", dtP: nullInt(2), want: "time(2)"},
		{name: "interval(4)", typ: "interval", dtP: nullInt(4), want: "interval(4)"},
		// bit: charLen があれば常に付与
		{name: "bit(8)", typ: "bit", charLen: nullInt(8), want: "bit(8)"},
		{name: "bit varying(64)", typ: "bit varying", charLen: nullInt(64), want: "bit varying(64)"},
		// 修飾子対象外型は素通し
		{name: "integer is unchanged", typ: "integer", numP: nullInt(32), want: "integer"},
		{name: "boolean is unchanged", typ: "boolean", want: "boolean"},
		{name: "uuid is unchanged", typ: "uuid", want: "uuid"},
		// 配列: 基底に修飾子を付けて [] を保持
		{name: "varchar(128)[]", typ: "varchar[]", charLen: nullInt(128), want: "varchar(128)[]"},
		{name: "numeric(10,2)[]", typ: "numeric[]", numP: nullInt(10), numS: nullInt(2), want: "numeric(10,2)[]"},
		{name: "timestamp(3)[]", typ: "timestamp[]", dtP: nullInt(3), want: "timestamp(3)[]"},
		// 配列で要素の修飾子が NULL（呼び出し側で COALESCE が無効）の場合は無修飾のまま
		{name: "text[] without any precision stays text[]", typ: "text[]", want: "text[]"},
	}
	_ = none // sql.NullInt64{} のゼロ値テスト用に確保（個別ケースの省略パラメータが暗黙に none）。
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := applyPGTypeModifier(c.typ, c.charLen, c.numP, c.numS, c.dtP)
			if got != c.want {
				t.Fatalf("applyPGTypeModifier(%q,...) = %q, want %q", c.typ, got, c.want)
			}
		})
	}
}

// TestPGArrayElementDisplay は内部 udt 名（`_<elem>`）→ data_type 寄り表示名の
// 純粋写像を直接検証する。`_` 抜きの未知名はそのまま返るフォールバックを
// 含めて固定する。
func TestPGArrayElementDisplay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		udt  string
		want string
	}{
		{"_int4", "integer"},
		{"_int8", "bigint"},
		{"_varchar", "varchar"},
		{"_timestamptz", "timestamptz"},
		{"_timestamp", "timestamp"},
		{"_timetz", "timetz"},
		{"_time", "time"},
		{"_text", "text"},
		// 未知の独自型は `_` のみ剥がして返す（独自 enum の配列など）
		{"_mood", "mood"},
		// 既に `_` 無しでも壊さない（堅牢性）
		{"int4", "integer"},
	}
	for _, c := range cases {
		t.Run(c.udt, func(t *testing.T) {
			t.Parallel()
			if got := pgArrayElementDisplay(c.udt); got != c.want {
				t.Fatalf("pgArrayElementDisplay(%q) = %q, want %q", c.udt, got, c.want)
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
