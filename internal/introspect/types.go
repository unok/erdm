package introspect

// 内部 DTO（package private）。ドライバ別アダプタ層と builder の境界を
// 担う一時形式で、永続化されない（design.md §"内部 DTO"）。
//
// builder.go はこの DTO 列を入力として *model.Schema を構築し、論理名の
// 物理名フォールバック（要件 8.6）／FK カーディナリティ決定（要件 6.1～
// 6.4）／スコープ外参照 FK の警告 + スキップ（要件 6.5）を一括で処理する。

// rawTable はドライバから取得したテーブル単位の情報。
type rawTable struct {
	// Name は物理テーブル名。
	Name string
	// Comment は DB から取得したテーブルコメント。空なら物理名がそのまま
	// 論理名としてフォールバックされる（要件 8.6）。
	Comment string
	// Columns は宣言順を保持するカラム列（要件 4.1）。
	Columns []rawColumn
	// PrimaryKey は主キー構成カラムの物理名を宣言順に保持する。
	PrimaryKey []string
	// ForeignKeys は外部キー定義。複合構成は構成カラム順序を維持する。
	ForeignKeys []rawForeignKey
	// Indexes は補助インデックス。主キー起源・UNIQUE 起源のものは
	// ドライバ側のクエリで除外済み（要件 7.1）。
	Indexes []rawIndex
}

// rawColumn はドライバから取得したカラム単位の情報。
type rawColumn struct {
	// Name は物理カラム名。
	Name string
	// Type は正規化済みの型表記（自動連番列はドライバ別の所定形式に置換済み、
	// 要件 4.7）。
	Type string
	// Comment はカラムコメント。空なら物理名フォールバック。
	Comment string
	// NotNull は NOT NULL 属性（要件 4.3）。
	NotNull bool
	// IsUnique は単一カラム UNIQUE 属性（要件 4.4）。
	IsUnique bool
	// Default はデフォルト値表現。自動連番列はドライバ層で空にクリア済み
	// （要件 4.7）。
	Default string
}

// rawForeignKey は外部キー定義。複合 FK は SourceColumns の長さで表現する。
type rawForeignKey struct {
	// SourceColumns は参照元（自テーブル）側のカラム物理名列。
	SourceColumns []string
	// TargetTable は参照先テーブルの物理名。
	TargetTable string
	// SourceUnique は単一カラム FK で参照元カラムが UNIQUE のとき true
	// （要件 6.3 のカーディナリティ判定に利用）。
	SourceUnique bool
}

// rawIndex は補助インデックス定義。
type rawIndex struct {
	// Name はインデックス名。
	Name string
	// Columns は構成カラムの物理名列。順序を維持する（要件 7.2）。
	Columns []string
	// IsUnique は UNIQUE インデックスかどうか（要件 7.3）。
	IsUnique bool
}
