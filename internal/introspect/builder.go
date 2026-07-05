package introspect

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/unok/erdm/internal/model"
)

// errEmptyIntrospection は取得テーブルが 0 件のときに返されるエラー
// （要件 3.5）。文言は要件本文に合わせて固定する。
var errEmptyIntrospection = errors.New("no user tables found in schema")

// logicalNameSanitizer は論理名のサニタイズ規則をまとめた 1 パス置換器
// （要件 8.7）。ナレッジ「操作の一覧性」に従い 1 か所に集約する。
//
//   - "/" → "／"（Mermaid／PlantUML 等の予約文字との衝突を回避）
//   - "\n" / "\r" → " "（複数行コメントを 1 行表示に正規化）
var logicalNameSanitizer = strings.NewReplacer("/", "／", "\n", " ", "\r", " ")

// buildSchema は内部 DTO 列をドメインモデル *model.Schema へ変換する純粋関数。
//
// 担当する責務（design.md §"要件トレーサビリティ" 3.5 / 4.x / 5.x / 6.x /
// 7.x / 8.4 ～ 8.7）:
//   - 物理名と DB コメントから論理名を解決し、空コメント時は物理名へ
//     フォールバック（要件 8.6 / ユーザー指示「基本コメントは論理名に」）。
//     具体ロジックは logicalName ヘルパに集約する。
//   - 主キー（単一・複合・無し）と補助インデックスを model.Table へ写像。
//   - FK のカーディナリティを参照元 NOT NULL / UNIQUE 性から決定する。
//   - 取得対象テーブルに含まれない参照先を持つ FK は関係を生成せず警告のみ
//     出す（要件 6.5）。警告は os.Stderr へ直接書き込む。要件 6.5 が
//     「標準エラーへ出力」を字義通り指定しているため、本層では io.Writer
//     抽象化を行わない（タスク 8.x 配線時に必要なら検討）。
//   - テーブル 0 件入力はエラーで返し、出力を生成しない（要件 3.5）。
func buildSchema(raws []rawTable, opts Options, knownTables map[string]struct{}) (*model.Schema, error) {
	if len(raws) == 0 {
		return nil, errEmptyIntrospection
	}
	tables := make([]model.Table, 0, len(raws))
	for _, raw := range raws {
		tables = append(tables, buildTable(raw, knownTables))
	}
	return &model.Schema{
		Title:  opts.Title,
		Tables: tables,
	}, nil
}

// buildTable は rawTable 1 件を model.Table に変換する。
// カラム変換／PK 添字構築／インデックス変換／FK 変換を順に適用する。
func buildTable(raw rawTable, knownTables map[string]struct{}) model.Table {
	columns, columnIndex := buildColumns(raw.Columns)
	primaryKeys := buildPrimaryKeys(raw.PrimaryKey, columnIndex, columns)
	indexes := buildIndexes(raw.Indexes)
	applyIndexRefs(columns, indexes)
	applyForeignKeys(columns, columnIndex, raw.ForeignKeys, knownTables)
	return model.Table{
		Name:        raw.Name,
		LogicalName: logicalName(raw.Name, raw.Comment),
		Columns:     columns,
		PrimaryKeys: primaryKeys,
		Indexes:     indexes,
	}
}

// buildColumns は rawColumn 列を model.Column 列に変換し、物理名 → 添字の
// 索引マップも併せて返す。索引マップは PK／FK 解決で利用する。
func buildColumns(raws []rawColumn) ([]model.Column, map[string]int) {
	columns := make([]model.Column, len(raws))
	index := make(map[string]int, len(raws))
	for i, raw := range raws {
		columns[i] = model.Column{
			Name:        raw.Name,
			LogicalName: logicalName(raw.Name, raw.Comment),
			Type:        raw.Type,
			AllowNull:   !raw.NotNull,
			IsUnique:    raw.IsUnique,
			Default:     raw.Default,
		}
		index[raw.Name] = i
	}
	return columns, index
}

// buildPrimaryKeys は主キー構成カラムの物理名列をカラム宣言順インデックス列に
// 変換し、対応カラムの IsPrimaryKey を立てる（要件 5.1 / 5.2）。
// 物理名が columns に存在しない場合は防御的に当該主キーを無視する。
func buildPrimaryKeys(pkColumns []string, columnIndex map[string]int, columns []model.Column) []int {
	if len(pkColumns) == 0 {
		return nil
	}
	out := make([]int, 0, len(pkColumns))
	for _, name := range pkColumns {
		idx, ok := columnIndex[name]
		if !ok {
			continue
		}
		out = append(out, idx)
		columns[idx].IsPrimaryKey = true
	}
	return out
}

// buildIndexes は補助インデックス DTO 列を model.Index 列に写像する
// （要件 7.2 / 7.3）。
func buildIndexes(raws []rawIndex) []model.Index {
	if len(raws) == 0 {
		return nil
	}
	out := make([]model.Index, len(raws))
	for i, raw := range raws {
		out[i] = model.Index{
			Name:     raw.Name,
			Columns:  raw.Columns,
			IsUnique: raw.IsUnique,
		}
	}
	return out
}

// applyIndexRefs は各カラムの IndexRefs を補完する。各カラムについて、
// 自身を含む補助インデックスの Table.Indexes 内番号を宣言順で格納する
// （internal/model.Column.IndexRefs 仕様）。
func applyIndexRefs(columns []model.Column, indexes []model.Index) {
	if len(indexes) == 0 {
		return
	}
	nameToIdx := make(map[string]int, len(columns))
	for i, c := range columns {
		nameToIdx[c.Name] = i
	}
	for refIdx, idx := range indexes {
		for _, colName := range idx.Columns {
			ci, ok := nameToIdx[colName]
			if !ok {
				continue
			}
			columns[ci].IndexRefs = append(columns[ci].IndexRefs, refIdx)
		}
	}
}

// applyForeignKeys は外部キー DTO 列をカラムへ反映する。
//
// 仕様（要件 6.1 ～ 6.5）:
//   - 参照先テーブルが knownTables に無い場合は os.Stderr へ警告を出力し、
//     当該 FK を生成しない（要件 6.5）。処理は継続する。
//   - 複合 FK は構成の先頭カラムのみに関係を付与する（要件 6.4）。
//   - カーディナリティは decideFKCardinality に集約。
func applyForeignKeys(columns []model.Column, columnIndex map[string]int, fks []rawForeignKey, knownTables map[string]struct{}) {
	for _, fk := range fks {
		if len(fk.SourceColumns) == 0 {
			continue
		}
		if _, ok := knownTables[fk.TargetTable]; !ok {
			fmt.Fprintf(os.Stderr, "skip foreign key referencing out-of-scope table: %s\n", fk.TargetTable)
			continue
		}
		headName := fk.SourceColumns[0]
		headIdx, ok := columnIndex[headName]
		if !ok {
			continue
		}
		isComposite := len(fk.SourceColumns) > 1
		notNull := !columns[headIdx].AllowNull
		// 複合 FK の場合、SourceUnique は単一カラム UNIQUE 性のみを指すため
		// 先頭カラムの UNIQUE 評価には用いない（要件 6.4）。
		unique := false
		if !isComposite {
			unique = fk.SourceUnique
		}
		cardSrc, cardDst := decideFKCardinality(notNull, unique, isComposite)
		columns[headIdx].FK = &model.FK{
			TargetTable:            fk.TargetTable,
			CardinalitySource:      cardSrc,
			CardinalityDestination: cardDst,
		}
	}
}

// decideFKCardinality は外部キーのカーディナリティを純粋関数として決定する
// （要件 6.1 / 6.2 / 6.3 / 6.4）。
//
// 引数:
//   - notNull: 参照元先頭カラムが NOT NULL か。
//   - unique: 単一カラム FK で参照元カラムが UNIQUE か（複合 FK では false）。
//   - isComposite: 複合 FK か（SourceColumns の長さ > 1）。
//
// 戻り値: (CardinalitySource, CardinalityDestination)。
// 採用根拠は research.md §3.4。要件 6.3 は「UNIQUE な単一カラム外部キー」を
// 一対一としており、NN 区別を要求しないため `0..1` / `1` 一律で採用する。
func decideFKCardinality(notNull, unique, isComposite bool) (string, string) {
	if isComposite {
		if notNull {
			return "1..*", "1"
		}
		return "0..*", "1"
	}
	if unique {
		return "0..1", "1"
	}
	if notNull {
		return "1..*", "1"
	}
	return "0..*", "1"
}

// logicalName はテーブル名・カラム名共通で使う論理名フォールバック関数。
//
// 仕様（要件 8.4 / 8.5 / 8.6 / 8.7）:
//   - comment が空文字列なら physical をそのまま返す（物理名フォールバック）。
//   - comment が空でなければスラッシュ "/" を全角スラッシュ "／" に、
//     改行（\n / \r）を半角スペースに置換して返す（サニタイズ）。
//
// テーブル・カラム双方で共通利用するため、ここに一元化する
// （ナレッジ「DRY」）。サニタイズは strings.NewReplacer による 1 パス置換で
// 実装し、複数の特殊文字混在ケースでも 1 度の走査で完結させる。
func logicalName(physical, comment string) string {
	if comment == "" {
		return physical
	}
	return logicalNameSanitizer.Replace(comment)
}
