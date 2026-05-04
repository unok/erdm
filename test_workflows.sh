#!/usr/bin/env bash
# GitHub Actions ワークフロー (.github/workflows/ci.yml, release.yml) の受け入れテスト。
# Issue #21「CI を github actions に変更」の要件 R1〜R7 を検証する。
# 真実のソース: もとの .circleci/config.yml と .takt/runs/.../reports/plan.md。
#
# 実行: bash test_workflows.sh
# 依存: yq (>=3), bash, grep
# 終了コード: 0=全テストパス / 1=失敗あり

set -u

# プロジェクトルート（このスクリプトの置き場所）に移動する。
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

CI_YML=".github/workflows/ci.yml"
RELEASE_YML=".github/workflows/release.yml"
CIRCLECI_YML=".circleci/config.yml"

PASS=0
FAIL=0
FAILED_TESTS=()

# Given-When-Then 構造のテストハーネス。
#  $1: テスト名 (Given/When/Then 形式の文)
#  $2: 評価する bash 式 (真なら pass)
assert() {
  local name="$1"
  local expr="$2"
  if eval "${expr}" >/dev/null 2>&1; then
    PASS=$((PASS + 1))
    printf "  [PASS] %s\n" "${name}"
  else
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("${name}")
    printf "  [FAIL] %s\n" "${name}"
  fi
}

# ============================================================================
# Section 1: ファイル配置 (R3, スコープ)
# ============================================================================
echo "[Section 1] ファイル配置"

assert "Given Issue #21, When migration is done, Then .github/workflows/ci.yml exists" \
  '[ -f "${CI_YML}" ]'

assert "Given Issue #21, When migration is done, Then .github/workflows/release.yml exists" \
  '[ -f "${RELEASE_YML}" ]'

assert "Given migration replaces CircleCI, When done, Then .circleci/config.yml is removed" \
  '[ ! -f "${CIRCLECI_YML}" ]'

# ファイルが存在しなければ以降のテストはスキップする (yq が落ちるため)
if [ ! -f "${CI_YML}" ] || [ ! -f "${RELEASE_YML}" ]; then
  echo ""
  echo "ワークフローファイルが未作成のため以降のテストをスキップします。"
  echo "  PASS=${PASS}  FAIL=${FAIL}"
  echo ""
  for t in "${FAILED_TESTS[@]}"; do
    echo "  - ${t}"
  done
  exit 1
fi

# ============================================================================
# Section 2: YAML 構文の妥当性
# ============================================================================
echo ""
echo "[Section 2] YAML 構文の妥当性"

assert "Given ci.yml, When parsed, Then yaml is syntactically valid" \
  'yq -r "." "${CI_YML}" >/dev/null'

assert "Given release.yml, When parsed, Then yaml is syntactically valid" \
  'yq -r "." "${RELEASE_YML}" >/dev/null'

# ============================================================================
# Section 3: ci.yml 振る舞い (R1, R4, R5)
# ============================================================================
echo ""
echo "[Section 3] ci.yml 振る舞い"

# トリガー: push to master と pull_request の両方が必要
# 注: yq に "on" キーを渡すと on/off 解釈されるため、'."on"' でクォートする。
assert "Given ci.yml, When triggered, Then push to master is configured" \
  'yq -r ".\"on\".push.branches[]" "${CI_YML}" 2>/dev/null | grep -qx "master"'

assert "Given ci.yml, When triggered, Then pull_request trigger is configured" \
  'yq -r ".\"on\" | keys[]" "${CI_YML}" 2>/dev/null | grep -qx "pull_request"'

# ランナー (R1)
assert "Given ci.yml, When job runs, Then runs-on is ubuntu-latest" \
  'yq -r ".jobs[].\"runs-on\"" "${CI_YML}" 2>/dev/null | grep -qx "ubuntu-latest"'

# TZ (R5)
assert "Given ci.yml, When job runs, Then env TZ is Asia/Tokyo" \
  'yq -r ".jobs[].env.TZ // .env.TZ" "${CI_YML}" 2>/dev/null | grep -qx "Asia/Tokyo"'

# Go セットアップ (R4): setup-go@v5 + go.mod 二重管理回避
assert "Given ci.yml, When setting up Go, Then actions/setup-go@v5 is used" \
  'yq -r ".jobs[].steps[].uses // empty" "${CI_YML}" 2>/dev/null | grep -qx "actions/setup-go@v5"'

assert "Given ci.yml, When setting up Go, Then go-version-file points to go.mod (no version hardcode)" \
  'yq -r ".jobs[].steps[].with.\"go-version-file\" // empty" "${CI_YML}" 2>/dev/null | grep -qx "go.mod"'

# checkout
assert "Given ci.yml, When job runs, Then actions/checkout@v4 is used" \
  'yq -r ".jobs[].steps[].uses // empty" "${CI_YML}" 2>/dev/null | grep -qx "actions/checkout@v4"'

# peg のインストールと実行 (R1)
assert "Given ci.yml, When building, Then peg is installed via go install" \
  'grep -q "go install github.com/pointlander/peg" "${CI_YML}"'

assert "Given ci.yml, When building, Then peg erdm.peg is executed" \
  'grep -q "peg erdm.peg" "${CI_YML}"'

# ビルド (R1)
assert "Given ci.yml, When building, Then go build ./... is executed" \
  'grep -q "go build \./\.\.\." "${CI_YML}"'

# 不要ツールが入っていない (アンチパターン: scope creep)
assert "Given ci.yml, When building, Then go-bindata is NOT installed (//go:embed への移行済み)" \
  '! grep -q "go-bindata" "${CI_YML}"'

assert "Given ci.yml, When triggered for build, Then gox is NOT installed (build job では不要)" \
  '! grep -q "go install github.com/mitchellh/gox" "${CI_YML}"'

# Go バージョン直書き禁止 (二重管理回避)
assert "Given ci.yml, When configuring Go, Then go-version is NOT hardcoded" \
  '! yq -r ".jobs[].steps[].with.\"go-version\" // empty" "${CI_YML}" 2>/dev/null | grep -E "^[0-9]"'

# ============================================================================
# Section 4: release.yml 振る舞い (R2, R4, R5, R6, R7)
# ============================================================================
echo ""
echo "[Section 4] release.yml 振る舞い"

# トリガー: タグ push のみ
assert "Given release.yml, When triggered, Then push tags filter is configured" \
  'yq -r ".\"on\".push.tags[]" "${RELEASE_YML}" 2>/dev/null | grep -q "v"'

assert "Given release.yml, When triggered, Then pull_request is NOT a trigger (release のみ)" \
  '! yq -r ".\"on\" | keys[]" "${RELEASE_YML}" 2>/dev/null | grep -qx "pull_request"'

# ランナー
assert "Given release.yml, When job runs, Then runs-on is ubuntu-latest" \
  'yq -r ".jobs[].\"runs-on\"" "${RELEASE_YML}" 2>/dev/null | grep -qx "ubuntu-latest"'

# TZ (R5)
assert "Given release.yml, When job runs, Then env TZ is Asia/Tokyo" \
  'yq -r ".jobs[].env.TZ // .env.TZ" "${RELEASE_YML}" 2>/dev/null | grep -qx "Asia/Tokyo"'

# Permissions (GitHub Release 公開には contents: write が必要)
assert "Given release.yml, When creating GitHub release, Then permissions.contents is write" \
  'yq -r ".jobs[].permissions.contents // .permissions.contents" "${RELEASE_YML}" 2>/dev/null | grep -qx "write"'

# Go セットアップ (R4)
assert "Given release.yml, When setting up Go, Then actions/setup-go@v5 is used" \
  'yq -r ".jobs[].steps[].uses // empty" "${RELEASE_YML}" 2>/dev/null | grep -qx "actions/setup-go@v5"'

assert "Given release.yml, When setting up Go, Then go-version-file points to go.mod" \
  'yq -r ".jobs[].steps[].with.\"go-version-file\" // empty" "${RELEASE_YML}" 2>/dev/null | grep -qx "go.mod"'

assert "Given release.yml, When job runs, Then actions/checkout@v4 is used" \
  'yq -r ".jobs[].steps[].uses // empty" "${RELEASE_YML}" 2>/dev/null | grep -qx "actions/checkout@v4"'

# Semver 厳格チェック (R7) - 二段防御の workflow 内 step
assert "Given release.yml, When tag arrives, Then semver regex check (^v(0|[1-9][0-9]*)(\\.(0|[1-9][0-9]*)){2}\$) is enforced" \
  'grep -F "^v(0|[1-9][0-9]*)(\\.(0|[1-9][0-9]*)){2}\$" "${RELEASE_YML}" >/dev/null'

# peg / gox のインストール (R2)
assert "Given release.yml, When preparing tools, Then peg is installed" \
  'grep -q "go install github.com/pointlander/peg" "${RELEASE_YML}"'

assert "Given release.yml, When preparing tools, Then gox is installed" \
  'grep -q "go install github.com/mitchellh/gox" "${RELEASE_YML}"'

assert "Given release.yml, When generating parser, Then peg erdm.peg is executed" \
  'grep -q "peg erdm.peg" "${RELEASE_YML}"'

# クロスコンパイル (R6) - 全 6 OS/Arch
assert "Given release.yml, When cross-compiling, Then linux/amd64 is included" \
  'grep -q "linux/amd64" "${RELEASE_YML}"'

assert "Given release.yml, When cross-compiling, Then linux/arm64 is included" \
  'grep -q "linux/arm64" "${RELEASE_YML}"'

assert "Given release.yml, When cross-compiling, Then darwin/amd64 is included" \
  'grep -q "darwin/amd64" "${RELEASE_YML}"'

assert "Given release.yml, When cross-compiling, Then darwin/arm64 is included" \
  'grep -q "darwin/arm64" "${RELEASE_YML}"'

assert "Given release.yml, When cross-compiling, Then windows/amd64 is included" \
  'grep -q "windows/amd64" "${RELEASE_YML}"'

assert "Given release.yml, When cross-compiling, Then windows/386 is included" \
  'grep -q "windows/386" "${RELEASE_YML}"'

# gox 出力フォーマット
assert "Given release.yml, When cross-compiling, Then gox -output dist/{{.Dir}}_{{.OS}}_{{.Arch}} is used" \
  'grep -F "dist/{{.Dir}}_{{.OS}}_{{.Arch}}" "${RELEASE_YML}" >/dev/null'

# Release 公開
assert "Given release.yml, When publishing, Then softprops/action-gh-release@v2 is used" \
  'yq -r ".jobs[].steps[].uses // empty" "${RELEASE_YML}" 2>/dev/null | grep -qx "softprops/action-gh-release@v2"'

assert "Given release.yml, When publishing, Then dist/* artifacts are uploaded" \
  'grep -q "dist/\*" "${RELEASE_YML}"'

assert "Given release.yml, When publishing, Then tag_name uses github.ref_name (no release_tag file read)" \
  'grep -q "github.ref_name" "${RELEASE_YML}"'

# 不要ツールが入っていない
assert "Given release.yml, When publishing, Then ghr is NOT installed (softprops 採用)" \
  '! grep -q "go install github.com/tcnksm/ghr" "${RELEASE_YML}"'

assert "Given release.yml, When building, Then go-bindata is NOT installed" \
  '! grep -q "go-bindata" "${RELEASE_YML}"'

# release_tag ファイルを CI で参照しない
assert "Given release.yml, When determining tag, Then release_tag file is NOT cat'd (github.ref_name 利用)" \
  '! grep -q "cat release_tag" "${RELEASE_YML}"'

# Go バージョン直書き禁止
assert "Given release.yml, When configuring Go, Then go-version is NOT hardcoded" \
  '! yq -r ".jobs[].steps[].with.\"go-version\" // empty" "${RELEASE_YML}" 2>/dev/null | grep -E "^[0-9]"'

# ============================================================================
# 結果集計
# ============================================================================
echo ""
echo "===================================="
echo "  PASS: ${PASS}"
echo "  FAIL: ${FAIL}"
echo "===================================="

if [ "${FAIL}" -gt 0 ]; then
  echo ""
  echo "失敗したテスト:"
  for t in "${FAILED_TESTS[@]}"; do
    echo "  - ${t}"
  done
  exit 1
fi

exit 0
