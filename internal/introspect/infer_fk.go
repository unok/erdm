package introspect

import (
	"strings"

	"github.com/jinzhu/inflection"

	"github.com/unok/erdm/internal/model"
)

// inferNamingConventionFKs は物理名規約から外部キー関係を補完する。
//
// 動機: 実 DB に FK 制約が貼られていないスキーマでも、`<entity>_id` 列名と
// 対応する複数形テーブル名（あるいは不可算名詞でそのまま）から
// 親子関係を機械的に導けることが多い。本関数はその経路を提供する。
//
// 規則（要件: ユーザー指示「テーブル(複数形) column (テーブル名の単数_id)
// をつなぐ」）:
//
//   - 対象列: 末尾が `_id`、かつ列自身に明示的 FK が無い、列名が `id` 単独でない。
//   - 単数→複数化は `github.com/jinzhu/inflection` の Plural を使用する
//     （不可算 `media` は `media`、`agency` は `agencies`、`person` は `people`
//     など辞書ベースで処理）。複数化の結果と一致するテーブルが無い場合は
//     `<base>` 自身も候補とする（テーブルが単数形で命名されている schema
//     に備えるため）。
//   - 自テーブル参照（target == 自テーブル）は推測しない。意図せず親子関係を
//     生成して図を破壊するリスクが大きいため、明示 FK を要求する。
//   - カーディナリティは `decideFKCardinality` を再利用（NN/UNIQUE 性に従う）。
//
// 副作用: schema.Tables[i].Columns[j].FK を直接書き換える。明示 FK が
// 既にある列は触らない（呼び出し側で applyForeignKeys が先に走る前提）。
func inferNamingConventionFKs(schema *model.Schema) {
	if schema == nil || len(schema.Tables) == 0 {
		return
	}
	tableNames := make(map[string]struct{}, len(schema.Tables))
	for _, t := range schema.Tables {
		tableNames[t.Name] = struct{}{}
	}
	for ti := range schema.Tables {
		tbl := &schema.Tables[ti]
		for ci := range tbl.Columns {
			col := &tbl.Columns[ci]
			if col.FK != nil {
				continue
			}
			target, ok := inferFKTarget(col.Name, tbl.Name, tableNames)
			if !ok {
				continue
			}
			cardSrc, cardDst := decideFKCardinality(!col.AllowNull, col.IsUnique, false)
			col.FK = &model.FK{
				TargetTable:            target,
				CardinalitySource:      cardSrc,
				CardinalityDestination: cardDst,
			}
		}
	}
}

// inferFKTarget は 1 列について命名規約による FK 参照先テーブル名を返す。
// 候補が無い／自参照になる場合は ("", false) を返す。
//
// 候補の優先順:
//  1. inflection.Plural(base)（標準的な英語規則 + 辞書ベースの不可算/不規則）
//  2. base 自身（テーブルが単数形命名のスキーマに備えるフォールバック）
func inferFKTarget(columnName, ownTableName string, tableNames map[string]struct{}) (string, bool) {
	if !strings.HasSuffix(columnName, "_id") {
		return "", false
	}
	base := strings.TrimSuffix(columnName, "_id")
	if base == "" {
		// 列名が `id` 単独だった場合。FK ではなく PK の規約。
		return "", false
	}
	for _, candidate := range []string{inflection.Plural(base), base} {
		if candidate == "" || candidate == ownTableName {
			continue
		}
		if _, ok := tableNames[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}
