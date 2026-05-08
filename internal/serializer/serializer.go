// Package serializer はドメインモデル `*model.Schema` を `.erdm` テキストへ
// 変換するリファレンス実装を提供する。
//
// 公開境界は Serialize 関数のみ。書式化ヘルパは小文字始まりで package private。
//
// 役割と非役割:
//
//   - 役割: 要件 7.10 の往復冪等性（Parse → Serialize → Parse → Serialize がバイト
//     一致）を担保する Go 単体テスト基盤。後続のクライアント側 TS シリアライザ
//     （タスク 7.3）と同一規則で出力するための単一の真実。
//   - 非役割: HTTP `PUT /api/schema` ハンドラからは呼び出されない。design.md §C4
//     / §6.2 のとおり、SPA がシリアライズして送信したテキストをサーバはバイト列
//     のまま保存する。サーバ側でモデル → テキストの再シリアライズは行わない。
//
// 正規化規則は research.md §3.5 に従う:
//
//   - 1 行目: `# Title: <Schema.Title>`
//   - テーブル間に空行 1 行
//   - テーブル宣言行: `<Name>[/<LogicalName>][ @groups[...]]`
//   - カラム属性順固定: `[NN]` → `[U]` → `[=<default>]` → `[-erd]`
//   - `@groups[...]` は要素を二重引用符で囲み、カンマ + 半角スペース 1 個で連結
//   - 独立コメント行（`//` 始まり）は保持しない（コメント保持はスコープ外）
package serializer

import (
	"bytes"

	"github.com/unok/erdm/internal/model"
)

// Serialize は `*model.Schema` を `.erdm` テキスト（UTF-8 バイト列）へ変換する。
//
// 戻り値の error は契約上保つが、現在の実装ではエラーを返さない（`bytes.Buffer.Write`
// は I/O エラーを返さないため）。スキーマの妥当性は呼び出し側の責任で
// `Schema.Validate()` を通してから渡すこと（Validate を Serialize 内では呼ばない）。
//
// 出力は要件 7.10 の往復冪等性を満たすことを保証する。すなわち:
//
//	parse(serialize(schema₀))  ≡  schema₀  (意味的同一性)
//	serialize(parse(serialize(schema₀)))  ==  serialize(schema₀)  (バイト一致)
//
// （独立コメント行・空行・属性順序などの自由度はスコープ外。）
func Serialize(s *model.Schema) ([]byte, error) {
	var buf bytes.Buffer
	writeTitle(&buf, s.Title)
	for i := range s.Tables {
		buf.WriteByte('\n')
		writeTable(&buf, &s.Tables[i])
	}
	return buf.Bytes(), nil
}
