package introspect

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestLogicalName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		physical string
		comment  string
		want     string
	}{
		{
			name:     "empty comment falls back to physical name",
			physical: "users",
			comment:  "",
			want:     "users",
		},
		{
			name:     "non-empty comment is adopted as-is",
			physical: "users",
			comment:  "ユーザー",
			want:     "ユーザー",
		},
		{
			name:     "slash is replaced with full-width slash",
			physical: "users",
			comment:  "識別子/ID",
			want:     "識別子／ID",
		},
		{
			name:     "newline is replaced with single space",
			physical: "users",
			comment:  "ユーザー\n一覧",
			want:     "ユーザー 一覧",
		},
		{
			name:     "carriage return is replaced with single space",
			physical: "users",
			comment:  "ユーザー\r一覧",
			want:     "ユーザー 一覧",
		},
		{
			name:     "all special chars mixed",
			physical: "users",
			comment:  "u/ser\nname\rid",
			want:     "u／ser name id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := logicalName(tt.physical, tt.comment)
			if got != tt.want {
				t.Errorf("logicalName(%q, %q) = %q; want %q", tt.physical, tt.comment, got, tt.want)
			}
		})
	}
}

func TestDecideFKCardinality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		notNull     bool
		unique      bool
		isComposite bool
		wantSrc     string
		wantDst     string
	}{
		{
			name:    "single nullable non-unique → many-to-one",
			wantSrc: "0..*", wantDst: "1",
		},
		{
			name:    "single not-null non-unique → mandatory many-to-one",
			notNull: true,
			wantSrc: "1..*", wantDst: "1",
		},
		{
			name:    "single not-null unique → one-to-one",
			notNull: true, unique: true,
			wantSrc: "0..1", wantDst: "1",
		},
		{
			name:    "single nullable unique → one-to-one",
			unique:  true,
			wantSrc: "0..1", wantDst: "1",
		},
		{
			name:        "composite not-null → mandatory many-to-one",
			notNull:     true,
			isComposite: true,
			wantSrc:     "1..*", wantDst: "1",
		},
		{
			name:        "composite nullable → many-to-one",
			isComposite: true,
			wantSrc:     "0..*", wantDst: "1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotSrc, gotDst := decideFKCardinality(tt.notNull, tt.unique, tt.isComposite)
			if gotSrc != tt.wantSrc || gotDst != tt.wantDst {
				t.Errorf("decideFKCardinality(%v, %v, %v) = (%q, %q); want (%q, %q)",
					tt.notNull, tt.unique, tt.isComposite, gotSrc, gotDst, tt.wantSrc, tt.wantDst)
			}
		})
	}
}

func TestBuildSchema_EmptyInputReturnsError(t *testing.T) {
	t.Parallel()

	got, err := buildSchema(nil, Options{}, nil)
	if got != nil {
		t.Errorf("expected nil schema, got %#v", got)
	}
	if !errors.Is(err, errEmptyIntrospection) {
		t.Errorf("expected errEmptyIntrospection, got %v", err)
	}
}

func TestBuildSchema_PhysicalFallbackWhenCommentIsEmpty(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "users",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "name", Type: "TEXT"},
			},
			PrimaryKey: []string{"id"},
		},
	}
	known := map[string]struct{}{"users": {}}

	schema, err := buildSchema(raws, Options{Title: "demo"}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.Title != "demo" {
		t.Errorf("Schema.Title = %q; want %q", schema.Title, "demo")
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("Schema.Tables length = %d; want 1", len(schema.Tables))
	}
	tbl := schema.Tables[0]
	if tbl.LogicalName != tbl.Name {
		t.Errorf("Table.LogicalName = %q; want fallback to Name %q", tbl.LogicalName, tbl.Name)
	}
	for _, col := range tbl.Columns {
		if col.LogicalName != col.Name {
			t.Errorf("Column %q LogicalName = %q; want fallback to Name", col.Name, col.LogicalName)
		}
	}
}

func TestBuildSchema_CompositePrimaryKey(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "order_items",
			Columns: []rawColumn{
				{Name: "order_id", Type: "INT", NotNull: true},
				{Name: "line_no", Type: "INT", NotNull: true},
				{Name: "qty", Type: "INT"},
			},
			PrimaryKey: []string{"order_id", "line_no"},
		},
	}
	known := map[string]struct{}{"order_items": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tbl := schema.Tables[0]
	if want := []int{0, 1}; !equalInts(tbl.PrimaryKeys, want) {
		t.Errorf("PrimaryKeys = %v; want %v", tbl.PrimaryKeys, want)
	}
	for i, col := range tbl.Columns {
		switch i {
		case 0, 1:
			if !col.IsPrimaryKey {
				t.Errorf("column[%d].IsPrimaryKey = false; want true", i)
			}
		default:
			if col.IsPrimaryKey {
				t.Errorf("column[%d].IsPrimaryKey = true; want false", i)
			}
		}
	}
}

func TestBuildSchema_OutOfScopeFKEmitsWarningAndDrops(t *testing.T) {
	stderr, restore := captureStderr(t)
	defer restore()

	raws := []rawTable{
		{
			Name: "orders",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "customer_id", Type: "INT", NotNull: true},
			},
			PrimaryKey: []string{"id"},
			ForeignKeys: []rawForeignKey{
				{SourceColumns: []string{"customer_id"}, TargetTable: "customers"},
			},
		},
	}
	known := map[string]struct{}{"orders": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, col := range schema.Tables[0].Columns {
		if col.FK != nil {
			t.Errorf("expected no FK to be generated, but column %q has FK %#v", col.Name, col.FK)
		}
	}
	got := stderr()
	if !strings.Contains(got, "skip foreign key referencing out-of-scope table: customers") {
		t.Errorf("warning message not found on stderr; got %q", got)
	}
}

func TestBuildSchema_SingleColumnUniqueFKBecomesOneToOne(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "profiles",
			Columns: []rawColumn{
				{Name: "user_id", Type: "INT", NotNull: true, IsUnique: true},
			},
			PrimaryKey: []string{"user_id"},
			ForeignKeys: []rawForeignKey{
				{SourceColumns: []string{"user_id"}, TargetTable: "users", SourceUnique: true},
			},
		},
		{
			Name: "users",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
			},
			PrimaryKey: []string{"id"},
		},
	}
	known := map[string]struct{}{"profiles": {}, "users": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	col := schema.Tables[0].Columns[0]
	if col.FK == nil {
		t.Fatalf("expected FK to be generated")
	}
	if col.FK.TargetTable != "users" {
		t.Errorf("FK.TargetTable = %q; want %q", col.FK.TargetTable, "users")
	}
	if col.FK.CardinalitySource != "0..1" || col.FK.CardinalityDestination != "1" {
		t.Errorf("FK cardinality = (%q, %q); want (%q, %q)",
			col.FK.CardinalitySource, col.FK.CardinalityDestination, "0..1", "1")
	}
}

func TestBuildSchema_CompositeFKAttachesOnlyToHeadColumn(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "order_items",
			Columns: []rawColumn{
				{Name: "order_id", Type: "INT", NotNull: true},
				{Name: "line_no", Type: "INT", NotNull: true},
			},
			PrimaryKey: []string{"order_id", "line_no"},
			ForeignKeys: []rawForeignKey{
				{
					SourceColumns: []string{"order_id", "line_no"},
					TargetTable:   "orders",
				},
			},
		},
		{
			Name: "orders",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "line_no", Type: "INT", NotNull: true},
			},
			PrimaryKey: []string{"id", "line_no"},
		},
	}
	known := map[string]struct{}{"order_items": {}, "orders": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tbl := schema.Tables[0]
	if tbl.Columns[0].FK == nil {
		t.Fatalf("head column expected to have FK")
	}
	if tbl.Columns[0].FK.CardinalitySource != "1..*" || tbl.Columns[0].FK.CardinalityDestination != "1" {
		t.Errorf("composite FK cardinality = (%q, %q); want (%q, %q)",
			tbl.Columns[0].FK.CardinalitySource, tbl.Columns[0].FK.CardinalityDestination, "1..*", "1")
	}
	if tbl.Columns[1].FK != nil {
		t.Errorf("non-head column expected to have nil FK, got %#v", tbl.Columns[1].FK)
	}
}

func TestBuildSchema_IndexesAndIndexRefs(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "users",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "email", Type: "TEXT"},
				{Name: "name", Type: "TEXT"},
			},
			PrimaryKey: []string{"id"},
			Indexes: []rawIndex{
				{Name: "idx_email", Columns: []string{"email"}, IsUnique: true},
				{Name: "idx_email_name", Columns: []string{"email", "name"}, IsUnique: false},
			},
		},
	}
	known := map[string]struct{}{"users": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tbl := schema.Tables[0]
	if len(tbl.Indexes) != 2 {
		t.Fatalf("Table.Indexes length = %d; want 2", len(tbl.Indexes))
	}
	if !tbl.Indexes[0].IsUnique || tbl.Indexes[1].IsUnique {
		t.Errorf("IsUnique propagation broken: %+v", tbl.Indexes)
	}

	emailIdx := -1
	nameIdx := -1
	for i, c := range tbl.Columns {
		switch c.Name {
		case "email":
			emailIdx = i
		case "name":
			nameIdx = i
		}
	}
	if !equalInts(tbl.Columns[emailIdx].IndexRefs, []int{0, 1}) {
		t.Errorf("email IndexRefs = %v; want [0,1]", tbl.Columns[emailIdx].IndexRefs)
	}
	if !equalInts(tbl.Columns[nameIdx].IndexRefs, []int{1}) {
		t.Errorf("name IndexRefs = %v; want [1]", tbl.Columns[nameIdx].IndexRefs)
	}
}

func TestBuildSchema_ColumnAttributesPropagated(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "users",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true, IsUnique: true},
				{Name: "name", Type: "TEXT", Default: "'anon'"},
				{Name: "memo", Type: "TEXT"},
			},
			PrimaryKey: []string{"id"},
		},
	}
	known := map[string]struct{}{"users": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cols := schema.Tables[0].Columns
	if cols[0].AllowNull || !cols[0].IsUnique {
		t.Errorf("id column: AllowNull=%v IsUnique=%v; want false,true", cols[0].AllowNull, cols[0].IsUnique)
	}
	if !cols[1].AllowNull || cols[1].Default != "'anon'" {
		t.Errorf("name column: AllowNull=%v Default=%q; want true,'anon'", cols[1].AllowNull, cols[1].Default)
	}
	if !cols[2].AllowNull || cols[2].Default != "" {
		t.Errorf("memo column: AllowNull=%v Default=%q; want true,''", cols[2].AllowNull, cols[2].Default)
	}
}

// equalInts は []int の浅い等価判定。テスト内の小型データでのみ利用する。
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// captureStderr は os.Stderr を一時的にパイプに差し替え、テスト中に書き込まれた
// 内容を文字列として取得するためのテストヘルパ。
func captureStderr(t *testing.T) (read func() string, restore func()) {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	var (
		mu       sync.Mutex
		captured strings.Builder
		done     = make(chan struct{})
	)
	go func() {
		defer close(done)
		buf, _ := io.ReadAll(r)
		mu.Lock()
		captured.Write(buf)
		mu.Unlock()
	}()

	read = func() string {
		_ = w.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		return captured.String()
	}
	restore = func() {
		os.Stderr = orig
		_ = r.Close()
	}
	return read, restore
}
