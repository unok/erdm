#!/usr/bin/env bash
# scripts/check-requirements-coverage.sh
#
# タスク 8.3「要件カバレッジの最終確認チェックリストを実装テスト群で参照可能
# にする」用の検証スクリプト。
#
# design.md の §要件トレーサビリティ表に記載された要件 ID（範囲表記
# `1.1〜1.5` を個別 ID に展開）と、リポジトリ内のテストファイルにある
# `Requirements: X.Y, Z.W` コメントで参照されている要件 ID を比較し、
# 参照漏れを検出した場合は非ゼロ終了する。
#
# 使い方:
#   bash scripts/check-requirements-coverage.sh
#   bash scripts/check-requirements-coverage.sh path/to/design.md
#
# 出力:
#   成功時は `OK: N requirement IDs are covered.` を stdout に出力。
#   失敗時は不足 ID を stderr に列挙して exit 1。
#
# 除外（ドキュメント専用）:
#   要件 3.1（リポジトリ構成）/ 8.5・8.6（README 更新）はテストではなく
#   構成・ドキュメントで充足する性質のため、テスト網羅性検証から除外する。
#   除外理由は EXCLUDED_DOC_ONLY 配列のコメントを参照。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DESIGN_PATH="${1:-$REPO_ROOT/.kiro/specs/elk-webui-migration/design.md}"

if [ ! -f "$DESIGN_PATH" ]; then
    echo "ERROR: design.md not found at: $DESIGN_PATH" >&2
    exit 2
fi

# UTF-8 全角チルダ（〜, U+301C）。range セパレータとして使われる。
# Python に頼らず安全に扱うためシェル変数経由で渡す。
WAVE_DASH=$'\xe3\x80\x9c'

# テストではなくリポジトリ構成・ドキュメントで充足する要件 ID。
# - 3.1: `internal/{...}` 6 パッケージ存在（リポジトリ構成全体）
# - 8.5: README 更新（ドキュメント）
# - 8.6: Doc 更新（ドキュメント）
EXCLUDED_DOC_ONLY=("3.1" "8.5" "8.6")

# expected: design.md §要件トレーサビリティ表から要件 ID を抽出 → 範囲展開
expected=$(awk -F'|' -v wave="$WAVE_DASH" '
{
    cell = $2
    gsub(/^[ \t]+|[ \t]+$/, "", cell)
    # 範囲（例: "1.1〜1.5"）
    if (match(cell, /^[0-9]+\.[0-9]+(.+)?[0-9]+\.[0-9]+$/)) {
        # 範囲セパレータが全角チルダかをチェック
        if (index(cell, wave) > 0) {
            n = split(cell, parts, wave)
            if (n != 2) next
            split(parts[1], a, ".")
            split(parts[2], b, ".")
            if (a[1] != b[1]) next
            for (i = a[2] + 0; i <= b[2] + 0; i++) print a[1] "." i
            next
        }
    }
    # 単一 ID（例: "1.6"）
    if (match(cell, /^[0-9]+\.[0-9]+$/)) {
        print cell
    }
}
' "$DESIGN_PATH" | sort -u)

if [ -z "$expected" ]; then
    echo "ERROR: no requirement IDs extracted from $DESIGN_PATH" >&2
    echo "       (check that the traceability table format is intact)" >&2
    exit 2
fi

# 除外 ID を除く
expected_filtered=$(echo "$expected" | grep -vxF -f <(printf '%s\n' "${EXCLUDED_DOC_ONLY[@]}") || true)

# found: テストファイルの `Requirements:` コメントから要件 ID を抽出
# Go テスト + フロントテスト + シェルテスト全てを対象にする。
found=$(grep -hroE 'Requirements:[^\r\n]+' \
    --include='*_test.go' \
    --include='*.test.ts' \
    --include='*.test.tsx' \
    "$REPO_ROOT/internal" \
    "$REPO_ROOT/cmd_test.go" \
    "$REPO_ROOT/cmd_compat_test.go" \
    "$REPO_ROOT/frontend/src" \
    2>/dev/null \
    | sed 's/Requirements://g' \
    | tr ',' '\n' \
    | tr -d ' ' \
    | grep -E '^[0-9]+\.[0-9]+$' \
    | sort -u || true)

# 不足 ID を計算
missing=$(comm -23 <(echo "$expected_filtered") <(echo "$found"))

if [ -n "$missing" ]; then
    {
        echo "ERROR: requirement IDs missing test coverage:"
        echo "$missing" | sed 's/^/  - /'
        echo ""
        echo "Add a 'Requirements: X.Y, Z.W' comment to a relevant test"
        echo "in internal/, cmd_test.go, cmd_compat_test.go, or frontend/src/"
        echo "to declare which requirement(s) the test verifies."
    } >&2
    exit 1
fi

count=$(echo "$expected_filtered" | wc -l)
echo "OK: $count requirement IDs are covered by tests (excluding doc-only IDs: ${EXCLUDED_DOC_ONLY[*]})."
