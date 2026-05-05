// Package parser は `.erdm` テキストを内部ドメインモデル（github.com/unok/erdm/internal/model.Schema）へ
// 変換するパーサ層を提供する。
//
// 公開境界は Parse 関数と ParseError 型のみ。PEG 自動生成物（Parser/Pretty/Size 等）は
// 副次的にパッケージ外からも参照可能だが、利用者は Parse のみを呼ぶことを期待する
// （tasks 2.3 / design.md §C3）。
package parser

import "github.com/unok/erdm/internal/model"

// Parse は `.erdm` 形式のバイト列をパースして *model.Schema を返す。
//
// パース成功時は (*model.Schema, nil) を返す。Schema.Validate() は呼ばないため、
// 不変条件の検証が必要な呼び出し側（HTTP ハンドラ等）は別途 Validate() する。
//
// 構文エラー時は (nil, *ParseError) を返す。エラー位置は 1-based の行・列で、
// HTTP API 経由でフロントへ渡すときに変換不要（要件 7.9 と整合）。
func Parse(src []byte) (*model.Schema, *ParseError) {
	p := &Parser{Buffer: string(src)}
	if err := p.Init(); err != nil {
		// peg ジェネレータが Init で失敗するのは内部のオプション設定エラーのみで
		// 利用者起因では到達しない。安全側で構造化エラーに包んで返す。
		return nil, &ParseError{Pos: 0, Line: 1, Column: 1, Message: err.Error()}
	}
	if err := p.Parse(); err != nil {
		// PEG が文法マッチに失敗した時点で parserBuilder.Err(pos, buffer) が呼ばれ、
		// parseErr に正確な行・列が積まれている。これを優先して返すことで API
		// 利用者（/api/schema 等）が常に 1:1 を見せられる挙動を避ける。
		if p.parserBuilder.parseErr != nil {
			return nil, p.parserBuilder.parseErr
		}
		return nil, &ParseError{Pos: 0, Line: 1, Column: 1, Message: err.Error()}
	}
	p.Execute()
	if p.parserBuilder.parseErr != nil {
		return nil, p.parserBuilder.parseErr
	}
	return p.parserBuilder.toSchema(), nil
}
