// Package html は *model.Schema から旧 CLI 互換の HTML レンダリングを行う。
//
// 公開境界は Render 関数のみ。テンプレートは embed.FS で同梱し、外部から
// 参照不可な状態でパッケージ内に閉じる（design.md §C7「テンプレート所有」）。
//
// 出力は旧 erdm CLI が `templates/html.tmpl` から生成していた HTML（PNG 埋め込み +
// テーブル一覧 + サイドバー）と同等の構造を持ち、要件 9.1 の後方互換を担保する。
// `PUT /api/schema` ハンドラからは呼ばれない（Web UI は SPA 経路で代替）。
package html

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"

	"github.com/unok/erdm/internal/model"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// htmlView はテンプレートに渡すビューモデル。
//
// *model.Schema を埋め込むことで Title/Tables/Groups を露出し、HTML 固有の
// 表示属性 ImageFilename を追加する。Schema 本体に表示属性を混ぜないため
// （ドメイン純粋性の保持）、ラップ構造体で渡す。
type htmlView struct {
	*model.Schema
	ImageFilename string
}

// Render は Schema を旧 CLI 互換の HTML へレンダリングして返す。
//
// imageFilename は HTML 内 <img src="..."> に埋め込む ERD 画像のファイル名で、
// 旧 CLI と同様に呼び出し側が明示的に渡す（Schema には含めない）。
//
// s が nil の場合はエラーを返す。テンプレート解析・実行で失敗した場合も
// その原因をラップしたエラーを返す（フォールバックは行わない）。
func Render(s *model.Schema, imageFilename string) ([]byte, error) {
	if s == nil {
		return nil, errors.New("html: schema is nil")
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("html: parse templates: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "html", htmlView{Schema: s, ImageFilename: imageFilename}); err != nil {
		return nil, fmt.Errorf("html: execute template: %w", err)
	}
	return buf.Bytes(), nil
}
