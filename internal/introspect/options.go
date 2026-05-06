package introspect

// Options は Introspect の入力を集約する値オブジェクト。
//
// 設計判断（design.md §"コンポーネントとインターフェース" / 要件 1.2 ～ 1.5,
// 9.5, 9.6）:
//   - DSN は必須。空のまま Introspect に渡された場合は呼び出し側が事前に
//     usage 出力 + 非ゼロ終了を行う（要件 1.6）。本パッケージはここでも
//     防御的にエラーを返す。
//   - Driver はゼロ値（DriverUnknown）の場合 DSN から推定する（要件 1.5）。
//   - Schema は --schema オプションの値。空なら PostgreSQL 既定 public、
//     MySQL 既定は接続先 DB とする（要件 3.3 / 3.4）。
//   - Title は --title オプションの値。空なら DSN から既定値を解決する
//     （要件 9.5 / 9.6）。具体ロジックは title.go の resolveTitle に集約する。
type Options struct {
	// Driver は --driver で明示指定されたドライバ種別。空文字列の場合は
	// DSN からの自動推定を行う（要件 1.5）。
	Driver Driver

	// DSN は --dsn の値。原文のまま保持し、ログ・エラー出力時は必ず
	// maskDSN を経由させる（要件 2.5 / 10.4）。
	DSN string

	// Schema は --schema の値。空文字列はドライバごとの既定動作に委ねる。
	Schema string

	// Title は --title の値。空文字列はドライバごとの既定値解決に委ねる。
	Title string
}
