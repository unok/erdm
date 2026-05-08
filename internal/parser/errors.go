package parser

import (
	"fmt"
	"strings"
)

// ParseError は `.erdm` テキストの構文エラーを構造化して保持する。
//
// Pos は入力バイト列の 0-based バイト位置、Line/Column はそれを変換した
// 1-based 行・列（IDE 慣習に合わせる。HTTP API 経由でフロントへ渡すときに
// 変換不要）。Message は人間可読のエラー説明。
type ParseError struct {
	Pos     int
	Line    int
	Column  int
	Message string
}

// Error は標準エラーインターフェースの実装。
func (e *ParseError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("parse error at line %d, column %d: %s", e.Line, e.Column, e.Message)
}

// newParseError は PEG の `pos`（バイトオフセット）と入力 buffer から
// 1-based 行・列を計算して ParseError を組み立てる。
//
// pos が buffer 長を超える場合は buffer 末尾位置に丸める。これは PEG の
// `Err(begin, buffer)` 呼び出しがマッチ末尾の begin を渡す前提に対し、
// EOT 位置を保護的に扱うため。
func newParseError(pos int, buffer, message string) *ParseError {
	if pos < 0 {
		pos = 0
	}
	if pos > len(buffer) {
		pos = len(buffer)
	}
	prefix := buffer[:pos]
	line := strings.Count(prefix, "\n") + 1
	lastNL := strings.LastIndexByte(prefix, '\n')
	column := pos - (lastNL + 1) + 1
	return &ParseError{
		Pos:     pos,
		Line:    line,
		Column:  column,
		Message: message,
	}
}
