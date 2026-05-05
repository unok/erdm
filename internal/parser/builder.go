package parser

import "strings"

// parserTable は PEG パース中に組み立てる中間テーブル表現。
// パース完了後 toSchema で model.Table へ変換される。
type parserTable struct {
	titleReal       string
	title           string
	columns         []parserColumn
	currentColumnID int
	primaryKeys     []int
	indexes         []parserIndex
	currentIndexID  int
	groups          []string
	hasGroupsDecl   bool
}

// parserColumn は PEG パース中に組み立てる中間カラム表現。
type parserColumn struct {
	titleReal    string
	title        string
	colType      string
	allowNull    bool
	isUnique     bool
	isPrimaryKey bool
	defaultExpr  string
	relation     parserRelation
	comments     []string
	indexIndexes []int
	withoutErd   bool
}

// parserRelation は FK 関係を保持する中間表現（model.FK の前段）。
type parserRelation struct {
	tableNameReal          string
	cardinalitySource      string
	cardinalityDestination string
}

// parserIndex は PEG パース中に組み立てる中間インデックス表現。
type parserIndex struct {
	title    string
	columns  []string
	isUnique bool
}

// parserBuilder は PEG レシーバが書き込む内部状態。Parser 構造体に埋め込まれる。
//
// 公開 API は parser.go の Parse のみであり、parserBuilder と各レシーバは
// すべて package private。外部から構造体・フィールドを直接操作させない
// （ナレッジ「パブリック API の公開範囲」遵守）。
type parserBuilder struct {
	title          string
	tables         []parserTable
	currentTableID int
	parseErr       *ParseError
}

// setTitle は `# Title:` 行から得られたタイトルを設定する。
func (p *parserBuilder) setTitle(t string) {
	p.title = t
}

// addTableTitleReal は新しいテーブル宣言を開始し、物理名を設定する。
func (p *parserBuilder) addTableTitleReal(t string) {
	p.tables = append(p.tables, parserTable{titleReal: t})
	p.currentTableID = len(p.tables) - 1
}

// addTableTitle は現在のテーブルに論理名を設定する。引用符で囲まれている
// 場合は除去する（既存パーサと同じ規則）。
func (p *parserBuilder) addTableTitle(t string) {
	t = strings.Trim(t, "\"")
	p.tables[p.currentTableID].title = t
}

// addPrimaryKey は主キーマーカー（`+` / `*`）を検出した際に呼ばれる。
// PEG の文法上、`<pkey>` は次の `<real_column_name>`（→ setColumnNameReal）の
// 直前で発火する。よって呼び出し時点の `len(tbl.columns)` は、続けて append
// される新カラムの添字と一致する。これを primaryKeys に積めば
// isPrimaryKeyIndex(colIndex) が任意の位置で正しく真を返せる。
//
// なお、引数 text は PEG 自動生成側のシグネチャ整合のために受け取るが、
// 値は常に "+" または "*"（pkey 文法定義）であり利用しない。
func (p *parserBuilder) addPrimaryKey(_ string) {
	tbl := &p.tables[p.currentTableID]
	tbl.primaryKeys = append(tbl.primaryKeys, len(tbl.columns))
}

// setColumnNameReal は新しいカラムを開始し、物理名を設定する。
// 旧パーサと同じく allowNull=true（NN 指定がなければ NULL 許容）で初期化する。
func (p *parserBuilder) setColumnNameReal(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns = append(tbl.columns, parserColumn{titleReal: t, allowNull: true})
	tbl.currentColumnID = len(tbl.columns) - 1
	tbl.columns[tbl.currentColumnID].isPrimaryKey = tbl.isPrimaryKeyIndex(tbl.currentColumnID)
}

// setColumnName は現在のカラムに論理名を設定する。引用符は除去する。
func (p *parserBuilder) setColumnName(t string) {
	t = strings.Trim(t, "\"")
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].title = t
}

// addColumnType は現在のカラムに型を設定する。
func (p *parserBuilder) addColumnType(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].colType = t
}

// setNotNull は現在のカラムを NOT NULL（allowNull=false）にする。
func (p *parserBuilder) setNotNull() {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].allowNull = false
}

// setUnique は現在のカラムに UNIQUE 制約を設定する。
func (p *parserBuilder) setUnique() {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].isUnique = true
}

// setColumnDefault は現在のカラムにデフォルト値式を設定する。
func (p *parserBuilder) setColumnDefault(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].defaultExpr = t
}

// setWithoutErd は現在のカラムを ERD 非表示扱い（`-erd` 属性付与）にする。
func (p *parserBuilder) setWithoutErd() {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].withoutErd = true
}

// setRelationSource は FK 関係の出発側カーディナリティ（左辺）を設定する。
// FK 関係の存在判定は relation.tableNameReal の有無で行う（toSchema 側）。
func (p *parserBuilder) setRelationSource(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].relation.cardinalitySource = t
}

// setRelationDestination は FK 関係の到着側カーディナリティ（右辺）を設定する。
func (p *parserBuilder) setRelationDestination(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].relation.cardinalityDestination = t
}

// setRelationTableNameReal は FK 関係の参照先テーブル物理名を設定する。
// この値が空でないとき toSchema は FK 値オブジェクトを生成する。
func (p *parserBuilder) setRelationTableNameReal(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].relation.tableNameReal = t
}

// addComment は現在のカラムにコメントを追加する。
func (p *parserBuilder) addComment(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.columns[tbl.currentColumnID].comments = append(tbl.columns[tbl.currentColumnID].comments, t)
}

// setIndexName は新しいインデックス宣言を開始し、物理名を設定する。
func (p *parserBuilder) setIndexName(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.indexes = append(tbl.indexes, parserIndex{title: t})
	tbl.currentIndexID = len(tbl.indexes) - 1
}

// setUniqueIndex は現在のインデックスを UNIQUE 指定にする。
func (p *parserBuilder) setUniqueIndex() {
	tbl := &p.tables[p.currentTableID]
	tbl.indexes[tbl.currentIndexID].isUnique = true
}

// setIndexColumn は現在のインデックスに対象カラム名を 1 件追加する。
// 同時に、対応する Column.indexIndexes にも逆参照を記録する。
// 文法上カラム未定義の名前が来ることは想定していないが、見つからなければ
// 解析エラーとして parseErr を立てる（旧パーサでは fmt.Println しつつ続行
// していたが、Fail Fast の方針で構造化エラー化する）。
func (p *parserBuilder) setIndexColumn(t string) {
	tbl := &p.tables[p.currentTableID]
	tbl.indexes[tbl.currentIndexID].columns = append(tbl.indexes[tbl.currentIndexID].columns, t)
	idx, ok := tbl.findColumnIndex(t)
	if !ok {
		if p.parseErr == nil {
			p.parseErr = &ParseError{
				Pos:     0,
				Line:    1,
				Column:  1,
				Message: "index references unknown column: " + t,
			}
		}
		return
	}
	tbl.columns[idx].indexIndexes = append(tbl.columns[idx].indexIndexes, tbl.currentIndexID)
}

// markGroupsDecl は現在のテーブルに `@groups[...]` 宣言が現れたことを記録する。
// 文法側で `(groups_decl space*)?` により単一回しか受理しないため、ここに
// 二度入ることは通常ない。Defense-in-depth として重複検出を残す（要件 2.9）。
func (p *parserBuilder) markGroupsDecl() {
	tbl := &p.tables[p.currentTableID]
	if tbl.hasGroupsDecl && p.parseErr == nil {
		p.parseErr = &ParseError{
			Pos:     0,
			Line:    1,
			Column:  1,
			Message: "duplicate @groups declaration on table " + tbl.titleReal,
		}
		return
	}
	tbl.hasGroupsDecl = true
}

// addGroup は `@groups[...]` 内の引用符付きグループ名を 1 件、現在のテーブルに
// 追加する。先頭が primary、それ以降が secondary（要件 2.7）。
// PEG の `<group_string>` は引用符を含めて捕捉するため、ここで Trim する。
func (p *parserBuilder) addGroup(t string) {
	t = strings.Trim(t, "\"")
	tbl := &p.tables[p.currentTableID]
	tbl.groups = append(tbl.groups, t)
}

// Err は PEG が文法レベルでマッチに失敗した位置を渡してくる際に呼ばれる。
// pos は失敗位置（バイトオフセット）、buffer は入力全体。最初の 1 件のみ保持。
func (p *parserBuilder) Err(pos int, buffer string) {
	if p.parseErr != nil {
		return
	}
	p.parseErr = newParseError(pos, buffer, "syntax error")
}

// containsInt は ints の中に v が含まれるかを返す（addPrimaryKey の旧仕様互換用）。
func containsInt(ints []int, v int) bool {
	for _, i := range ints {
		if i == v {
			return true
		}
	}
	return false
}

// isPrimaryKeyIndex は colIndex が当該テーブルの primaryKeys に含まれるかを返す。
// 旧パーサの Table.isPrimaryKey と同義。
func (t *parserTable) isPrimaryKeyIndex(colIndex int) bool {
	return containsInt(t.primaryKeys, colIndex)
}

// findColumnIndex は物理名 name を持つカラムの添字を返す。見つからない場合は
// (0, false)。
func (t *parserTable) findColumnIndex(name string) (int, bool) {
	for i, c := range t.columns {
		if c.titleReal == name {
			return i, true
		}
	}
	return 0, false
}
