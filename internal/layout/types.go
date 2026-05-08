// Package layout は手動配置の座標を JSON ファイルに永続化する。
//
// 公開境界は Load / Save / Layout / Position / LoadError のみ
// （design.md §C9）。標準ライブラリのみに依存し、internal/model など
// 上位ドメインへの依存は持たない（座標は単純な map で表現可能なため）。
//
// JSON フォーマット（design.md §論理データモデル）:
//
//	{
//	  "<table_name>": { "x": float, "y": float },
//	  ...
//	}
//
// 利用箇所:
//   - HTTP API（タスク 6.3）の `GET /api/layout` / `PUT /api/layout`
//   - SPA 起動時の座標読み込み（タスク 7.4）
package layout

import "fmt"

// Layout はテーブル物理名 → 座標 の対応を表す。JSON のトップレベルが
// オブジェクト直下のキー値ペアとなる形式（design.md §論理データモデル）に
// 合わせ、型エイリアスではなく独立型として宣言して Marshal/Unmarshal の
// 挙動を一意に固定する。
type Layout map[string]Position

// Position はキャンバス座標（左上基点）を表す。JSON フィールド名は小文字
// `x` / `y`（design.md §論理データモデル）。
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// LoadError は Load 中に発生した「呼び出し側で区別すべきエラー」を表す
// 構造化エラー（要件 6.6）。ファイル不存在は LoadError ではなく空 Layout
// として返すため、LoadError が返るのは「ファイルが存在するが読めない／
// JSON として破損している」場合に限られる。
//
// 呼び出し側は `errors.As(err, &le)` で型判別できるよう error インター
// フェースを実装する。
type LoadError struct {
	Path  string
	Cause string
}

// Error は LoadError の error インターフェース実装。`Path` と `Cause` を
// 含む人間可読なメッセージを返す。
func (e *LoadError) Error() string {
	return fmt.Sprintf("layout load error at %s: %s", e.Path, e.Cause)
}
