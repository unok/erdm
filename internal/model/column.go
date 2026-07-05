package model

// Column はテーブルが持つ 1 カラムを表すエンティティ。
//
// Name は物理名（識別子）、LogicalName は論理名（人間可読の表示名）。
// FK が nil でないとき、そのカラムは外部キーであり、Schema.Validate で
// FK.TargetTable が同 Schema 内に存在することが要求される。
// IndexRefs は Table.Indexes のうち、このカラムを含むものの添字一覧を保持する。
type Column struct {
	Name         string
	LogicalName  string
	Type         string
	AllowNull    bool
	IsUnique     bool
	IsPrimaryKey bool
	Default      string
	Comments     []string
	WithoutErd   bool
	FK           *FK
	IndexRefs    []int
}

// HasRelation は外部キー関係を持つかどうかを返す。FK が設定されていれば真。
//
// テンプレート（internal/dot/html/ddl）と TS 側シリアライザはすべて本メソッドを
// 参照する単一表現に統一する（旧 erdm.go の Column.IsForeignKey フィールドの
// 置き換え）。
func (c *Column) HasRelation() bool {
	return c.FK != nil
}

// HasDefault はデフォルト値が設定されているかどうかを返す。
// 旧テンプレート参照名 {{.HasDefaultSetting}} の置換先（design.md §テンプレ対応表）。
func (c *Column) HasDefault() bool {
	return len(c.Default) > 0
}

// HasComment はコメントが 1 件以上あるかどうかを返す。
// 旧テンプレート参照名 {{.HasComment}} の派生メソッド。
func (c *Column) HasComment() bool {
	return len(c.Comments) > 0
}
