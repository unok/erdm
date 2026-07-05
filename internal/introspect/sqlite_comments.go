package introspect

import (
	"regexp"
	"strings"
)

// sqlite_comments.go は CREATE TABLE 原文（`sqlite_master.sql`）からカラム宣言
// 行末の行コメント（`-- ...`）を抽出する純粋関数を提供する（要件 8.3）。
//
// 抽出戦略:
//   - 入力 DDL を改行で分割し、各行ごとに「最初の `--` 以降をコメント」、
//     `--` より前を「コード部」とみなす。
//   - コード部から「`(` ／ `,` の直後（または行頭）に現れる識別子」を全列挙し、
//     その中で `knownColumns` に一致する最後（最右）の識別子をその行の対象
//     カラムとして採用する。これにより `CREATE TABLE t (id INTEGER, -- ...`
//     のような同一行記法も、`  id INTEGER, -- ...` のような 1 行 1 カラム記法も
//     同じ規則で解釈できる。
//   - 対象カラムが特定できない／`knownColumns` にない場合はその行を読み飛ばす
//     （要件 8.6 の物理名フォールバックに委ねる、graceful degradation）。
//
// 既知の限界（要件 8.3 の「抽出可能な範囲で」を字義通り解釈）:
//   - 文字列リテラル内の `--` を誤検出する可能性がある。誤検出時は該当行の
//     コメント領域が誤って切り出されるが、対象カラムが knownColumns にあれば
//     誤コメントが採用される。本ツールは要件 8.6 の物理名フォールバックで
//     最終的な見栄えを保つ設計で、ここでは特別な救済を行わない。
//   - 複数行コメント（`/* ... */`）は対象外。
//   - テーブル直前または宣言行末のテーブルコメントは対象外（要件 8.3 が
//     「カラム宣言行末の行コメント」のみを主対象としているため）。
//
// 識別子のクオート方言:
//   - `"name"`（標準 SQL）／ `` `name` ``（MySQL 互換）／ `[name]`（MS SQL 互換）
//     は SQLite でいずれも有効。エスケープシーケンス（`""` 内の `""`）は
//     本ツールでは扱わない（実 DDL に現れる頻度が低く、要件範囲外）。

// sqliteIdentifierAfterDelimiter は `(` または `,` の直後（または行頭）に現れる
// 識別子を捕捉する正規表現。行頭マッチは `(?m)^` ではなく `^` を使い、行ごとに
// 切り出した文字列に対して FindAllStringSubmatch で適用する。
var sqliteIdentifierAfterDelimiter = regexp.MustCompile(`(?:^|[(,])\s*("[^"]+"|` + "`" + `[^` + "`" + `]+` + "`" + `|\[[^\]]+\]|[A-Za-z_][A-Za-z0-9_]*)`)

// extractSQLiteColumnComments は CREATE TABLE 原文からカラム宣言行末の行コメント
// を抽出し、物理カラム名 → コメントの map を返す純粋関数（要件 8.3）。
//
// 抽出不能なカラム（DDL に行コメントが無い／識別子が `knownColumns` に無い等）
// はマップに含まれない。呼び出し側はキー存在チェックで分岐する。
func extractSQLiteColumnComments(ddl string, knownColumns []string) map[string]string {
	out := map[string]string{}
	if ddl == "" || len(knownColumns) == 0 {
		return out
	}
	known := make(map[string]bool, len(knownColumns))
	for _, c := range knownColumns {
		known[c] = true
	}
	for _, line := range strings.Split(ddl, "\n") {
		col, comment, ok := extractSQLiteCommentFromLine(line, known)
		if !ok {
			continue
		}
		out[col] = comment
	}
	return out
}

// extractSQLiteCommentFromLine は 1 行から (対象カラム名, コメント本文, 抽出可否)
// を返す純粋ヘルパ。最初の `--` 以降をコメントとし、それより前のコード部から
// 「`(` または `,` の直後（または行頭）に現れる識別子」のうち knownColumns に
// 一致する最右の識別子を対象カラムとして採用する。
func extractSQLiteCommentFromLine(line string, known map[string]bool) (string, string, bool) {
	commentStart := strings.Index(line, "--")
	if commentStart < 0 {
		return "", "", false
	}
	codePart := line[:commentStart]
	commentText := strings.TrimSpace(line[commentStart+2:])
	matches := sqliteIdentifierAfterDelimiter.FindAllStringSubmatch(codePart, -1)
	col := ""
	for _, m := range matches {
		cand := unquoteSQLiteIdentifier(m[1])
		if known[cand] {
			col = cand
		}
	}
	if col == "" {
		return "", "", false
	}
	return col, commentText, true
}

// unquoteSQLiteIdentifier は `"name"` ／ `` `name` `` ／ `[name]` のクオートを
// 取り除いた識別子を返す。クオート無しはそのまま返す。
func unquoteSQLiteIdentifier(token string) string {
	if len(token) < 2 {
		return token
	}
	first := token[0]
	last := token[len(token)-1]
	switch {
	case first == '"' && last == '"':
		return token[1 : len(token)-1]
	case first == '`' && last == '`':
		return token[1 : len(token)-1]
	case first == '[' && last == ']':
		return token[1 : len(token)-1]
	}
	return token
}
