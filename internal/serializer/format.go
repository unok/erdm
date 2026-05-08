package serializer

import (
	"bytes"
	"strings"

	"github.com/unok/erdm/internal/model"
)

const (
	columnIndent  = "    "      // カラム宣言行のインデント（半角スペース 4）
	commentIndent = "     # "   // カラムコメント行のインデント（4 スペース + ` # `）
	indexIndent   = "    index" // インデックス宣言行の先頭（カラムと同じ深さに `index` を含める）
)

// writeTitle は `# Title: <title>\n` を書き込む。タイトルが空文字列でも形式は維持する。
func writeTitle(buf *bytes.Buffer, title string) {
	buf.WriteString("# Title: ")
	buf.WriteString(title)
	buf.WriteByte('\n')
}

// writeTable は 1 テーブルを書き込む。出力は宣言行 + カラム行 + インデックス行の順。
//
// テーブル間の空行は呼び出し側 (Serialize) の責任で挿入する（このヘルパは
// 末尾に空行を付けない）。
func writeTable(buf *bytes.Buffer, t *model.Table) {
	writeTableHeader(buf, t)
	for ci := range t.Columns {
		writeColumn(buf, &t.Columns[ci])
	}
	for ii := range t.Indexes {
		writeIndex(buf, &t.Indexes[ii])
	}
}

// writeTableHeader は `<Name>[/<LogicalName>][ @groups[...]]\n` を書き込む。
func writeTableHeader(buf *bytes.Buffer, t *model.Table) {
	buf.WriteString(t.Name)
	if t.LogicalName != "" {
		buf.WriteByte('/')
		buf.WriteString(formatNameLiteral(t.LogicalName))
	}
	if len(t.Groups) > 0 {
		buf.WriteByte(' ')
		writeGroupsDecl(buf, t.Groups)
	}
	buf.WriteByte('\n')
}

// writeGroupsDecl は `@groups["A", "B", ...]` 形式を書き込む。要素は二重引用符で
// 囲み、カンマ + 半角スペース 1 個で連結する（要件 2.4）。
func writeGroupsDecl(buf *bytes.Buffer, groups []string) {
	buf.WriteString(`@groups[`)
	for i, g := range groups {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteByte('"')
		buf.WriteString(g)
		buf.WriteByte('"')
	}
	buf.WriteByte(']')
}

// writeColumn は 1 カラムの宣言行と付随コメント行を書き込む。
//
//	<indent>[+]<name>[/<logical>] [<type>][NN][U][=<default>][-erd][ <fk>]\n
//	<commentIndent><comment>\n  (Comments の各要素について)
func writeColumn(buf *bytes.Buffer, c *model.Column) {
	buf.WriteString(columnIndent)
	if c.IsPrimaryKey {
		buf.WriteByte('+')
	}
	buf.WriteString(c.Name)
	if c.LogicalName != "" {
		buf.WriteByte('/')
		buf.WriteString(formatNameLiteral(c.LogicalName))
	}
	buf.WriteString(" [")
	buf.WriteString(c.Type)
	buf.WriteByte(']')
	writeColumnFlags(buf, c)
	if c.HasRelation() {
		buf.WriteByte(' ')
		writeRelation(buf, c.FK)
	}
	buf.WriteByte('\n')
	for _, cm := range c.Comments {
		buf.WriteString(commentIndent)
		buf.WriteString(cm)
		buf.WriteByte('\n')
	}
}

// writeColumnFlags は固定順 `[NN] → [U] → [=<default>] → [-erd]` で属性を書き込む。
func writeColumnFlags(buf *bytes.Buffer, c *model.Column) {
	if !c.AllowNull {
		buf.WriteString("[NN]")
	}
	if c.IsUnique {
		buf.WriteString("[U]")
	}
	if c.HasDefault() {
		buf.WriteString("[=")
		buf.WriteString(escapeDefaultExpr(c.Default))
		buf.WriteByte(']')
	}
	if c.WithoutErd {
		buf.WriteString("[-erd]")
	}
}

// writeRelation は FK を `<src>--<dst> <target>` 形式で書き込む。
//
// CardinalitySource / CardinalityDestination が空文字列の場合でも形式は維持する
// （文法上は両方任意。空のまま出力しても再パース時に同じ FK 値オブジェクトが
// 構築される）。
func writeRelation(buf *bytes.Buffer, fk *model.FK) {
	buf.WriteString(fk.CardinalitySource)
	buf.WriteString("--")
	buf.WriteString(fk.CardinalityDestination)
	buf.WriteByte(' ')
	buf.WriteString(fk.TargetTable)
}

// writeIndex は 1 インデックスの宣言行を書き込む。
//
//	<indent>index <Name> (<col1>, <col2>, ...)[ unique]\n
func writeIndex(buf *bytes.Buffer, idx *model.Index) {
	buf.WriteString(indexIndent)
	buf.WriteByte(' ')
	buf.WriteString(idx.Name)
	buf.WriteString(" (")
	for i, col := range idx.Columns {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(col)
	}
	buf.WriteByte(')')
	if idx.IsUnique {
		buf.WriteString(" unique")
	}
	buf.WriteByte('\n')
}

// escapeDefaultExpr は `[=...]` の値部分に現れる `]` を `\]` にエスケープする。
//
// PEG `default` 規則は `'\\]' / (![\r\n\]] .)` の選択で `\]` を `]` のエスケープと
// 解釈する。Parse 側は setColumnDefault で `\]` → `]` にアンエスケープして意味値
// を保持しているため、Serialize 側ではここで対称に再エスケープする（要件 7.10
// の往復冪等性）。PostgreSQL 配列 default `'{}'::integer[]` のような `]` を含む
// 式を含めて round-trip させるための前提処理。
func escapeDefaultExpr(v string) string {
	return strings.ReplaceAll(v, `]`, `\]`)
}

// formatNameLiteral は論理名を `.erdm` の table_name / column_name 規則で表現する。
//
// PEG 文法 `('"' (![\t\r\n"] .)+ '"') / (![\t\r\n/ ] .)+` に従い、空白・タブ・
// 改行・`/` を含む場合は二重引用符で囲み、それ以外は無引用で出力する。
func formatNameLiteral(name string) string {
	if needsQuotedLiteral(name) {
		return `"` + name + `"`
	}
	return name
}

// needsQuotedLiteral は table_name / column_name の無引用形が許されないかを返す。
func needsQuotedLiteral(name string) bool {
	return strings.ContainsAny(name, " \t\r\n/")
}
