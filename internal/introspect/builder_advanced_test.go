// builder_advanced_test.go はタスク 9.4（ドメインモデル変換の単体テスト）で
// 追加した網羅ケースを集約する。基本 6 ケース（builder_test.go の
// TestLogicalName ／ TestDecideFKCardinality ／ TestBuildSchema_*）に対し、
// エッジ条件（主キー無し ／ 自動連番列の正規化伝播 ／ 複合 FK 構造 ／
// 警告メッセージ構造 ／ サニタイズ横断 ／ 往復冪等性）を本ファイルへ分離した。
//
// 分離理由:
//   - builder_test.go の合計行数が 700 行を超え、目安 300 行を大きく逸脱した。
//   - 既存ケースは「契約固定の最小集合」、本ファイルは「9.4 で追加した
//     エッジ条件と往復冪等性」という責務の差で分割すると見通しがよい。
//   - 本ファイルは parser ／ serializer 依存を持つため、依存方向が単純な
//     既存 builder_test.go から切り離すと依存解析もしやすい。
//
// 共有ヘルパ（captureStderr ／ equalInts）は同パッケージ内の builder_test.go
// で定義されており、本ファイルはそれをそのまま参照する。
//
// _Requirements: 4.7, 5.1, 5.2, 5.3, 6.1, 6.2, 6.3, 6.4, 6.5, 8.4, 8.5, 8.6, 8.7_

package introspect

import (
	"strings"
	"testing"

	"github.com/unok/erdm/internal/parser"
	"github.com/unok/erdm/internal/serializer"
)

// TestBuildSchema_NoPrimaryKey は主キー無しテーブルでも正常変換され、
// `Table.PrimaryKeys` が空・全カラムの `IsPrimaryKey` が false で
// 維持されることを固定する（要件 5.3）。
func TestBuildSchema_NoPrimaryKey(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "audit_log",
			Columns: []rawColumn{
				{Name: "ts", Type: "TIMESTAMP", NotNull: true},
				{Name: "msg", Type: "TEXT"},
			},
			// PrimaryKey は空のまま（主キー無し）。
		},
	}
	known := map[string]struct{}{"audit_log": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tbl := schema.Tables[0]
	if len(tbl.PrimaryKeys) != 0 {
		t.Errorf("PrimaryKeys = %v; want empty", tbl.PrimaryKeys)
	}
	for _, col := range tbl.Columns {
		if col.IsPrimaryKey {
			t.Errorf("column %q: IsPrimaryKey = true; want false (no PK table)", col.Name)
		}
	}
}

// TestBuildSchema_AutoIncrementColumnPropagatesNormalizedTypeAndEmptyDefault は
// ドライバ層が自動連番列のデフォルト値を空クリアし、型を所定の連番型に
// 正規化済みであるという前提を builder が破壊しないことを固定する
// （要件 4.7）。本テストは入力 DTO 段階で「正規化済み」状態を作って投入し、
// builder がそれを変更せず Column へ写像することを確認する。
func TestBuildSchema_AutoIncrementColumnPropagatesNormalizedTypeAndEmptyDefault(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "users",
			Columns: []rawColumn{
				// 自動連番列：ドライバ層で Type=bigserial, Default=""（クリア済み）。
				{Name: "id", Type: "bigserial", NotNull: true, IsUnique: true, Default: ""},
				{Name: "name", Type: "TEXT"},
			},
			PrimaryKey: []string{"id"},
		},
	}
	known := map[string]struct{}{"users": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id := schema.Tables[0].Columns[0]
	if id.Type != "bigserial" {
		t.Errorf("auto-increment column Type = %q; want %q (driver-normalized)", id.Type, "bigserial")
	}
	if id.Default != "" {
		t.Errorf("auto-increment column Default = %q; want empty (driver-cleared)", id.Default)
	}
	if !id.IsPrimaryKey || id.AllowNull {
		t.Errorf("auto-increment column flags: IsPrimaryKey=%v AllowNull=%v; want true,false",
			id.IsPrimaryKey, id.AllowNull)
	}
}

// TestBuildSchema_CompositeFKTargetTableOnHead は複合外部キーの先頭カラムに
// 付与される FK の TargetTable と、非先頭カラムが nil 維持されることを
// 名前指定で具体的に検証する（要件 6.4）。既存 TestBuildSchema_CompositeFKAttachesOnlyToHeadColumn
// は Cardinality 検証が中心。本テストは「TargetTable が先頭カラムに付与され、
// 非先頭カラムには付与されない」という構造の側を固定する。
func TestBuildSchema_CompositeFKTargetTableOnHead(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "shipment_lines",
			Columns: []rawColumn{
				{Name: "shipment_id", Type: "INT", NotNull: true},
				{Name: "line_no", Type: "INT", NotNull: true},
				{Name: "qty", Type: "INT"},
			},
			PrimaryKey: []string{"shipment_id", "line_no"},
			ForeignKeys: []rawForeignKey{
				{
					SourceColumns: []string{"shipment_id", "line_no"},
					TargetTable:   "shipments",
				},
			},
		},
		{
			Name: "shipments",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "line_no", Type: "INT", NotNull: true},
			},
			PrimaryKey: []string{"id", "line_no"},
		},
	}
	known := map[string]struct{}{"shipment_lines": {}, "shipments": {}}

	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tbl := schema.Tables[0]
	head := tbl.Columns[0]
	if head.FK == nil {
		t.Fatalf("head column %q expected to have FK", head.Name)
	}
	if head.FK.TargetTable != "shipments" {
		t.Errorf("head column FK.TargetTable = %q; want %q", head.FK.TargetTable, "shipments")
	}
	for i := 1; i < len(tbl.Columns); i++ {
		if tbl.Columns[i].FK != nil {
			t.Errorf("non-head column %q has FK %#v; want nil", tbl.Columns[i].Name, tbl.Columns[i].FK)
		}
		// `qty` は FK の構成カラムでもないので当然 FK を持たないことも合わせて確認。
	}
}

// TestBuildSchema_OutOfScopeFKWarningIncludesTargetTableName は警告メッセージの
// 構造（プレフィックスとターゲットテーブル名）を厳密に検証する（要件 6.5）。
// 既存 TestBuildSchema_OutOfScopeFKEmitsWarningAndDrops は文字列含有のみを
// 確認するが、本テストは「skip foreign key referencing out-of-scope table:」
// プレフィックスと改行終端の存在を契約として固定する。
func TestBuildSchema_OutOfScopeFKWarningIncludesTargetTableName(t *testing.T) {
	read, restore := captureStderr(t)
	defer restore()

	raws := []rawTable{
		{
			Name: "orders",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "external_id", Type: "INT"},
			},
			PrimaryKey: []string{"id"},
			ForeignKeys: []rawForeignKey{
				{SourceColumns: []string{"external_id"}, TargetTable: "external_systems"},
			},
		},
	}
	known := map[string]struct{}{"orders": {}}

	if _, err := buildSchema(raws, Options{}, known); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := read()
	const wantPrefix = "skip foreign key referencing out-of-scope table: "
	if !strings.Contains(got, wantPrefix+"external_systems") {
		t.Errorf("warning %q should contain %q", got, wantPrefix+"external_systems")
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), "external_systems") {
		t.Errorf("warning %q should end with target table name", got)
	}
}

// TestBuildSchema_LogicalNameSanitizationOnTableAndColumn はテーブル名とカラム名
// の両方で同一サニタイズ規則（"/" → "／"・改行 → " "）が適用されることを
// 確認する（要件 8.4 / 8.5 / 8.7）。logicalName ヘルパが共通利用される
// 設計（builder.go コメント参照）の回帰検出を担う。
func TestBuildSchema_LogicalNameSanitizationOnTableAndColumn(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name:    "users",
			Comment: "ユーザー/管理\nテーブル",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "name", Type: "TEXT", Comment: "氏名/フルネーム\n表示用"},
				// 空コメントは物理名フォールバック（要件 8.6）。
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
	tbl := schema.Tables[0]
	if want := "ユーザー／管理 テーブル"; tbl.LogicalName != want {
		t.Errorf("Table.LogicalName = %q; want %q", tbl.LogicalName, want)
	}
	if want := "氏名／フルネーム 表示用"; tbl.Columns[1].LogicalName != want {
		t.Errorf("Column[name].LogicalName = %q; want %q", tbl.Columns[1].LogicalName, want)
	}
	if tbl.Columns[2].LogicalName != tbl.Columns[2].Name {
		t.Errorf("Column[memo].LogicalName = %q; want fallback to physical %q",
			tbl.Columns[2].LogicalName, tbl.Columns[2].Name)
	}
}

// TestApplyFKSourceUnique_SinglePKAsSourceTreatedAsUnique は、参照元の単一カラムが
// テーブルの単独主キーであるとき、PK が暗黙に UNIQUE である性質を踏まえて
// `SourceUnique=true` が立つことを固定する（要件 6.3 の補強）。
//
// PK 構成カラムは `IsUnique` フィールドが立てられないため、UNIQUE 起源
// （UNIQUE 制約・単一カラム UNIQUE インデックス）を持たない単独 PK が
// 単一カラム FK の参照元になっているケースで、`applyFKSourceUnique` が
// 誤って 1 対多と判定してしまう不具合を回帰検出する。
func TestApplyFKSourceUnique_SinglePKAsSourceTreatedAsUnique(t *testing.T) {
	t.Parallel()

	raw := rawTable{
		Name: "user_profiles",
		Columns: []rawColumn{
			{Name: "user_id", Type: "INT", NotNull: true},
			{Name: "bio", Type: "TEXT"},
		},
		PrimaryKey: []string{"user_id"},
		ForeignKeys: []rawForeignKey{
			{SourceColumns: []string{"user_id"}, TargetTable: "users"},
		},
	}
	applyFKSourceUnique(&raw)
	if !raw.ForeignKeys[0].SourceUnique {
		t.Fatalf("single-column PK FK source should be marked unique, got SourceUnique=false")
	}
}

// TestBuildSchema_SinglePKFKYields1To1Cardinality は、参照元が単独 PK である
// 単一カラム FK の出力カーディナリティが要件 6.3 の `0..1--1` になることを
// builder 経由で固定する。`applyFKSourceUnique` の単独 PK 反映が
// `decideFKCardinality` に伝播し、最終的な FK 表現が 1 対 1 になる経路を担保する。
func TestBuildSchema_SinglePKFKYields1To1Cardinality(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name: "user_profiles",
			Columns: []rawColumn{
				{Name: "user_id", Type: "INT", NotNull: true},
				{Name: "bio", Type: "TEXT"},
			},
			PrimaryKey: []string{"user_id"},
			ForeignKeys: []rawForeignKey{
				// 単独 PK が単一カラム FK の参照元。要件 6.3 の典型ケース。
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
	known := map[string]struct{}{"user_profiles": {}, "users": {}}
	schema, err := buildSchema(raws, Options{}, known)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fk := schema.Tables[0].Columns[0].FK
	if fk == nil {
		t.Fatalf("expected FK on user_profiles.user_id")
	}
	if fk.CardinalitySource != "0..1" || fk.CardinalityDestination != "1" {
		t.Fatalf("cardinality = %q--%q; want 0..1--1", fk.CardinalitySource, fk.CardinalityDestination)
	}
}

// TestBuildSchema_RoundTripThroughSerializerAndParser は buildSchema の出力を
// 既存 serializer.Serialize → parser.Parse → serializer.Serialize の経路に
// 流して 2 度目の Serialize 結果が 1 度目とバイト一致することを確認する
// （要件 7.10 の往復冪等性、ナレッジ「契約の再パース可能性」）。
//
// 本テストは builder の出力が既存パーサで受理可能な構造を保つことの
// 最小ケース。フルカバーは統合テスト 10.x の責務だが、本層でも
// 「物理名 + 論理名フォールバック + 主キー + FK + インデックス」の最小構成で
// 往復が壊れていないことを担保する。
func TestBuildSchema_RoundTripThroughSerializerAndParser(t *testing.T) {
	t.Parallel()

	raws := []rawTable{
		{
			Name:    "users",
			Comment: "ユーザー",
			Columns: []rawColumn{
				{Name: "id", Type: "bigserial", NotNull: true, IsUnique: true},
				{Name: "name", Type: "TEXT", Comment: "氏名"},
			},
			PrimaryKey: []string{"id"},
		},
		{
			Name: "orders",
			Columns: []rawColumn{
				{Name: "id", Type: "INT", NotNull: true},
				{Name: "user_id", Type: "INT", NotNull: true},
			},
			PrimaryKey: []string{"id"},
			ForeignKeys: []rawForeignKey{
				{SourceColumns: []string{"user_id"}, TargetTable: "users"},
			},
		},
	}
	known := map[string]struct{}{"users": {}, "orders": {}}

	schema, err := buildSchema(raws, Options{Title: "round-trip"}, known)
	if err != nil {
		t.Fatalf("buildSchema: %v", err)
	}
	if err := schema.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	first, err := serializer.Serialize(schema)
	if err != nil {
		t.Fatalf("first Serialize: %v", err)
	}
	parsed, perr := parser.Parse(first)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	second, err := serializer.Serialize(parsed)
	if err != nil {
		t.Fatalf("second Serialize: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("round trip mismatch:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
