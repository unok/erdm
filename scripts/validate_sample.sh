#!/usr/bin/env bash
# scripts/validate_sample.sh
#
# erdm CLI（render / serve）が `.erdm` DSL を仕様通りに処理することを、
# `doc/sample/validation_basic.erdm` / `doc/sample/validation_full.erdm` を
# 入力にエンドツーエンドで検証する CI/開発者向け検証ハーネス。
#
# 本ファイルはバッチ 1 の納品物として、以下の基盤ユーティリティのみを
# 提供する（メインフローと検証ブロックは後続バッチで追加）。
#   - 環境検出（detect_environment）
#   - 必須前提条件チェック（check_prerequisites）
#   - 一時ディレクトリ管理（setup_workdir / cleanup）
#   - 空きポート探索（find_free_port）
#   - アサート集約（assert_eq / assert_contains / assert_file_exists /
#     assert_exit_code / assert_status）
#   - スキップ判定（should_skip_block_dot / should_skip_block_serve /
#     should_skip_psql / should_skip_sqlite3）
#
# `</dev/tcp/...` を利用するため bash 4.x 以降を要求する。

set -euo pipefail

# --------------------------------------------------------------------------
# 定数
# --------------------------------------------------------------------------

# REPO_ROOT は本スクリプトの 1 階層上を基準に決定する（scripts/ 配下から呼ばれる前提）。
# `${BASH_SOURCE[0]}` を使うことで `bash scripts/validate_sample.sh` 直接実行と
# `source scripts/validate_sample.sh`（テスト目的の sourcing）の双方で正しく解決する。
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SAMPLE_BASIC="$REPO_ROOT/doc/sample/validation_basic.erdm"
SAMPLE_FULL="$REPO_ROOT/doc/sample/validation_full.erdm"
SPA_INDEX="$REPO_ROOT/frontend/dist/index.html"

# ポート探索範囲。18080 起点で 50 候補まで走査する（design.md §C3）。
PORT_BASE=18080
PORT_RANGE_SIZE=50

# サーバ起動レディ判定の上限秒数（後続バッチの ServerFixture で利用）。
SERVER_READY_TIMEOUT_SEC=5

# --------------------------------------------------------------------------
# グローバル状態（set -u 下で安全に参照できるよう初期化）
# --------------------------------------------------------------------------

# 環境検出結果（"true" / "false" 文字列で保持）。detect_environment が更新する。
has_dot="false"
has_psql="false"
has_sqlite3="false"
has_jq="false"
has_curl="false"
has_spa_dist="false"

# 一時作業ディレクトリ。setup_workdir が mktemp で確定する。
WORK=""

# 起動中サーバの PID。start_server / stop_server が更新する。
SERVER_PID=""

# 検証結果カウンタ。assert_* / record_skip が更新する。
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# erdm バイナリの絶対パス。check_prerequisites が `command -v erdm` の結果で更新する。
# PATH 制限テスト時もこの絶対パス経由で起動できるよう、解決責務をここに一元化する。
ERDM_BIN=""

# --------------------------------------------------------------------------
# 環境検出（タスク 2.1）
# --------------------------------------------------------------------------

# detect_environment は外部ツール / SPA 同梱物の有無を検出して
# グローバル has_* に格納する。値の変換責務はここで一元化する。
detect_environment() {
    has_dot=$(_command_present dot)
    has_psql=$(_command_present psql)
    has_sqlite3=$(_command_present sqlite3)
    has_jq=$(_command_present jq)
    has_curl=$(_command_present curl)
    if [[ -f "$SPA_INDEX" ]]; then
        has_spa_dist="true"
    else
        has_spa_dist="false"
    fi
}

_command_present() {
    if command -v "$1" >/dev/null 2>&1; then
        echo "true"
    else
        echo "false"
    fi
}

# --------------------------------------------------------------------------
# 必須前提条件チェック（タスク 2.1）
# --------------------------------------------------------------------------

# check_prerequisites は detect_environment 後に呼び、
# 不足があれば原因と解決手順を stderr に出して非ゼロ終了する。
# 必須は curl / SPA 同梱 / 入力サンプル 2 種 / erdm バイナリ。
check_prerequisites() {
    local missing=0
    if [[ "$has_curl" != "true" ]]; then
        echo "fatal: curl not found in PATH; required for HTTP checks" >&2
        missing=1
    fi
    if [[ "$has_spa_dist" != "true" ]]; then
        echo "fatal: $SPA_INDEX not found; run 'make frontend' first" >&2
        missing=1
    fi
    if [[ ! -f "$SAMPLE_BASIC" ]]; then
        echo "fatal: $SAMPLE_BASIC not found" >&2
        missing=1
    fi
    if [[ ! -f "$SAMPLE_FULL" ]]; then
        echo "fatal: $SAMPLE_FULL not found" >&2
        missing=1
    fi
    if ! command -v erdm >/dev/null 2>&1; then
        echo "fatal: erdm binary not found in PATH; run 'make build' and ensure PATH includes the binary" >&2
        missing=1
    else
        # PATH 制限テスト時にも同一バイナリを起動できるよう絶対パスを保持する。
        ERDM_BIN="$(command -v erdm)"
    fi
    if (( missing != 0 )); then
        exit 2
    fi
}

# --------------------------------------------------------------------------
# 一時ディレクトリと終了時クリーンアップ（タスク 2.2）
# --------------------------------------------------------------------------

# setup_workdir は mktemp で一意な作業ディレクトリを作成し、
# trap で cleanup を EXIT/INT/TERM に登録する。WORK 確定後に trap を張ることで
# 「WORK が空のまま rm -rf される」事故を防ぐ。
setup_workdir() {
    WORK="$(mktemp -d -t erdm_validate.XXXXXX)"
    trap cleanup EXIT INT TERM
}

# cleanup はサーバプロセスと一時ディレクトリを安全に解放する。
# 多重起動・既停止・既削除のいずれの状態でも副作用を起こさないことが要件 6.7。
cleanup() {
    if [[ -n "${SERVER_PID:-}" ]]; then
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            kill "$SERVER_PID" 2>/dev/null || true
            wait "$SERVER_PID" 2>/dev/null || true
        fi
    fi
    if [[ -n "${WORK:-}" && -d "$WORK" ]]; then
        rm -rf "$WORK"
    fi
}

# --------------------------------------------------------------------------
# 空きポート探索（タスク 2.3）
# --------------------------------------------------------------------------

# find_free_port は PORT_BASE から PORT_RANGE_SIZE 個の候補を順に走査し、
# 最初に見つかった空きポートを stdout に出す。python3 等の外部依存は使わず、
# bash 標準の `</dev/tcp/...` 接続試行の **失敗** を空き判定とする。
find_free_port() {
    local port
    local end=$(( PORT_BASE + PORT_RANGE_SIZE ))
    for (( port = PORT_BASE; port < end; port++ )); do
        if ! (echo > "/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1; then
            echo "$port"
            return 0
        fi
    done
    echo "fatal: no free port available in range $PORT_BASE..$(( end - 1 ))" >&2
    return 1
}

# --------------------------------------------------------------------------
# アサート集約（タスク 2.4）
# --------------------------------------------------------------------------
# 失敗時は FAIL_COUNT を加算し stderr に診断行を出すが exit はしない。
# スクリプト終了時に FAIL_COUNT を参照して非ゼロ終了するのは後続バッチの責務。

assert_eq() {
    local req="$1" label="$2" expected="$3" actual="$4"
    if [[ "$expected" == "$actual" ]]; then
        PASS_COUNT=$(( PASS_COUNT + 1 ))
        return 0
    fi
    echo "[FAIL] req=$req $label: expected='$expected' actual='$actual'" >&2
    FAIL_COUNT=$(( FAIL_COUNT + 1 ))
    return 1
}

assert_contains() {
    local req="$1" label="$2" haystack="$3" needle="$4"
    if [[ "$haystack" == *"$needle"* ]]; then
        PASS_COUNT=$(( PASS_COUNT + 1 ))
        return 0
    fi
    echo "[FAIL] req=$req $label: needle='$needle' not found in actual" >&2
    FAIL_COUNT=$(( FAIL_COUNT + 1 ))
    return 1
}

assert_file_exists() {
    local req="$1" label="$2" path="$3"
    if [[ -f "$path" ]]; then
        PASS_COUNT=$(( PASS_COUNT + 1 ))
        return 0
    fi
    echo "[FAIL] req=$req $label: expected file='$path' not found" >&2
    FAIL_COUNT=$(( FAIL_COUNT + 1 ))
    return 1
}

assert_exit_code() {
    local req="$1" label="$2" expected="$3" actual="$4"
    if [[ "$expected" == "$actual" ]]; then
        PASS_COUNT=$(( PASS_COUNT + 1 ))
        return 0
    fi
    echo "[FAIL] req=$req $label: expected exit=$expected actual exit=$actual" >&2
    FAIL_COUNT=$(( FAIL_COUNT + 1 ))
    return 1
}

assert_status() {
    local req="$1" label="$2" expected="$3" actual="$4"
    if [[ "$expected" == "$actual" ]]; then
        PASS_COUNT=$(( PASS_COUNT + 1 ))
        return 0
    fi
    echo "[FAIL] req=$req $label: expected status=$expected actual status=$actual" >&2
    FAIL_COUNT=$(( FAIL_COUNT + 1 ))
    return 1
}

# --------------------------------------------------------------------------
# スキップ判定（タスク 2.5）
# --------------------------------------------------------------------------
# 要件 6.8 の文言「のみ」に厳密準拠し、dot 不在時は block_render_dot と
# block_serve の両方をスキップする（design.md §C2）。
# psql / sqlite3 不在時は当該 assert を個別にスキップする。

should_skip_block_dot() {
    [[ "$has_dot" != "true" ]]
}

should_skip_block_serve() {
    [[ "$has_dot" != "true" ]]
}

should_skip_psql() {
    [[ "$has_psql" != "true" ]]
}

should_skip_sqlite3() {
    [[ "$has_sqlite3" != "true" ]]
}

# record_skip はスキップ理由を stdout に明示し、SKIP_COUNT を加算する。
# タスク 2.5 の AC「対象要件 ID を含む明示ログを出力し、当該検証を成功扱いとする」
# を満たす（スクリプト全体の終了コード判定では失敗扱いにしない）。
record_skip() {
    local req="$1" reason="$2"
    echo "[SKIP] req=$req $reason"
    SKIP_COUNT=$(( SKIP_COUNT + 1 ))
}

# --------------------------------------------------------------------------
# JSON 妥当性チェック（タスク 4 の補助）
# --------------------------------------------------------------------------

# _validate_json は引数のファイルパスが妥当な JSON かを検査する。
# `jq` 在時は `jq -e .`、不在時は「先頭文字 { または [、末尾文字 } または ]」の
# 粗フォールバック検証で代替する（要件 3.2 は構造の厳密性は要求していない）。
_validate_json() {
    local path="$1"
    if [[ "$has_jq" == "true" ]]; then
        jq -e . <"$path" >/dev/null 2>&1
        return $?
    fi
    local first last
    first="$(head -c 1 "$path" 2>/dev/null || true)"
    last="$(tail -c 1 "$path" 2>/dev/null || true)"
    [[ ( "$first" == "{" || "$first" == "[" ) && ( "$last" == "}" || "$last" == "]" ) ]]
}

# --------------------------------------------------------------------------
# render(dot) 検証ブロック（タスク 3 / 要件 2.1-2.9, 6.3, 6.8）
# --------------------------------------------------------------------------

# block_render_dot は構文網羅サンプルを `erdm` の DOT 形式で描画し、
# 5 種出力ファイル生成・DOT 再描画・PG/SQLite DDL の構文検査までを直列実行する。
# `dot` 不在時はブロック全体をスキップする（要件 6.8）。
block_render_dot() {
    if should_skip_block_dot; then
        record_skip "2.1-2.9" "dot command not found; render(dot) block skipped"
        return 0
    fi
    local out="$WORK/render_dot"
    mkdir -p "$out"

    # 要件 2.1: render 実行が終了コード 0
    local exit_code=0
    "$ERDM_BIN" -output_dir "$out" "$SAMPLE_FULL" >"$WORK/render_dot.stdout" 2>"$WORK/render_dot.stderr" || exit_code=$?
    assert_exit_code "2.1" "render(dot) exit code" 0 "$exit_code"

    # 要件 2.2-2.6: 5 種出力ファイルが生成される
    assert_file_exists "2.2" "DOT output"          "$out/validation_full.dot"
    assert_file_exists "2.3" "PNG output"          "$out/validation_full.png"
    assert_file_exists "2.4" "HTML output"         "$out/validation_full.html"
    assert_file_exists "2.5" "PostgreSQL DDL"      "$out/validation_full.pg.sql"
    assert_file_exists "2.6" "SQLite DDL"          "$out/validation_full.sqlite3.sql"

    # 要件 2.7: 生成 DOT を Graphviz で再描画して終了コード 0
    if [[ -f "$out/validation_full.dot" ]]; then
        local dot_exit=0
        dot -Tpng -o /dev/null "$out/validation_full.dot" >/dev/null 2>"$WORK/render_dot.dot_redraw.stderr" || dot_exit=$?
        assert_exit_code "2.7" "dot redraw" 0 "$dot_exit"
    else
        # 2.2 で既に FAIL を出しているため、ここでは追加 FAIL を出さず情報のみ。
        echo "[INFO] req=2.7 skipped due to missing DOT input (see req=2.2)" >&2
    fi

    # 要件 2.8: 生成 PostgreSQL DDL の構文検査（psql 不在 OR DB 接続不可ならスキップ）
    if should_skip_psql; then
        record_skip "2.8" "psql command not found"
    elif ! psql --no-psqlrc -c 'SELECT 1' >/dev/null 2>&1; then
        record_skip "2.8" "psql connect failed; no PostgreSQL instance reachable"
    elif [[ -f "$out/validation_full.pg.sql" ]]; then
        # 一時 DB は使わず、現在の接続先に対して 1 トランザクションで実行→ROLLBACK 相当の効果を狙う。
        # ON_ERROR_STOP=on で構文エラー時に非ゼロ終了する。
        local pg_exit=0
        psql --no-psqlrc --set ON_ERROR_STOP=on \
             -c 'BEGIN;' \
             -f "$out/validation_full.pg.sql" \
             -c 'ROLLBACK;' \
             >/dev/null 2>"$WORK/render_dot.pg.stderr" || pg_exit=$?
        assert_exit_code "2.8" "psql syntax check" 0 "$pg_exit"
    fi

    # 要件 2.9: 生成 SQLite DDL をメモリ DB に投入（sqlite3 不在ならスキップ）
    if should_skip_sqlite3; then
        record_skip "2.9" "sqlite3 command not found"
    elif [[ -f "$out/validation_full.sqlite3.sql" ]]; then
        local sq_exit=0
        sqlite3 ":memory:" ".read $out/validation_full.sqlite3.sql" \
            >/dev/null 2>"$WORK/render_dot.sqlite.stderr" || sq_exit=$?
        assert_exit_code "2.9" "sqlite3 syntax check" 0 "$sq_exit"
    fi
}

# --------------------------------------------------------------------------
# render(elk) 検証ブロック（タスク 4 / 要件 3.1-3.5, 6.4）
# --------------------------------------------------------------------------

# block_render_elk は ELK 形式の標準出力モード / 出力ディレクトリ指定モード /
# PATH 制限下動作の 3 軸を直列に検証する。`dot` 不在時もスキップしない（要件 9.4）。
block_render_elk() {
    local out="$WORK/render_elk"
    mkdir -p "$out"
    mkdir -p "$WORK/empty_path"

    # 要件 3.1: 出力ディレクトリ非指定で標準出力に ELK JSON が出て終了コード 0
    local stdout_path="$WORK/elk_stdout.json"
    local exit_code=0
    "$ERDM_BIN" --format=elk "$SAMPLE_FULL" >"$stdout_path" 2>"$WORK/render_elk.stderr" || exit_code=$?
    assert_exit_code "3.1" "render(elk) stdout mode exit code" 0 "$exit_code"

    # 要件 3.2: 取得した標準出力が JSON として妥当
    if [[ -s "$stdout_path" ]]; then
        local json_exit=0
        _validate_json "$stdout_path" || json_exit=$?
        assert_exit_code "3.2" "stdout is valid JSON" 0 "$json_exit"
    else
        # 3.1 で FAIL するパスでは追加 FAIL を出さない。
        echo "[INFO] req=3.2 skipped due to empty stdout (see req=3.1)" >&2
    fi

    # 要件 3.3 / 3.4: 出力ディレクトリ指定で ELK JSON ファイルが生成され、stdout は空
    local stdout2="$WORK/elk_stdout_dir.json"
    local exit2=0
    "$ERDM_BIN" --format=elk -output_dir "$out" "$SAMPLE_FULL" \
        >"$stdout2" 2>"$WORK/render_elk_dir.stderr" || exit2=$?
    assert_exit_code "3.3" "render(elk) dir mode exit code" 0 "$exit2"
    assert_file_exists "3.3" "ELK JSON file" "$out/validation_full.elk.json"
    local stdout2_size
    stdout2_size="$(wc -c <"$stdout2" | tr -d ' ')"
    assert_eq "3.4" "stdout is empty when -output_dir is set" "0" "$stdout2_size"

    # 要件 3.5: PATH 制限で dot 不在の状態でも ELK 形式は成功する
    local exit3=0
    PATH="$WORK/empty_path" "$ERDM_BIN" --format=elk "$SAMPLE_FULL" \
        >"$WORK/elk_no_dot.json" 2>"$WORK/render_elk_no_dot.stderr" || exit3=$?
    assert_exit_code "3.5" "render(elk) succeeds without dot in PATH" 0 "$exit3"
}

# --------------------------------------------------------------------------
# サーバ起動・停止フィクスチャ（タスク 5.1 / 要件 4.1, 6.5, 6.7）
# --------------------------------------------------------------------------

# start_server <port> <schema> [extra_flags...] はサーバをバックグラウンド起動し、
# HTTP 経由のレディ判定を所定タイムアウトでポーリングする。レディ確認後に return。
# stdout / stderr はファイルへリダイレクトし、デバッグ時に参照可能とする。
start_server() {
    local port="$1" schema="$2"
    shift 2
    : >"$WORK/server.stdout"
    : >"$WORK/server.stderr"
    "$ERDM_BIN" serve --port="$port" --listen=127.0.0.1 "$@" "$schema" \
        >"$WORK/server.stdout" 2>"$WORK/server.stderr" &
    SERVER_PID=$!

    # レディ判定: SERVER_READY_TIMEOUT_SEC を 0.5 秒間隔でポーリング。
    local deadline=$(( SERVER_READY_TIMEOUT_SEC * 2 ))
    local i
    for (( i = 0; i < deadline; i++ )); do
        # プロセスが先に死んでいたら早期失敗。
        if ! kill -0 "$SERVER_PID" 2>/dev/null; then
            echo "fatal: server process exited during startup; stderr:" >&2
            cat "$WORK/server.stderr" >&2 || true
            SERVER_PID=""
            return 1
        fi
        local code
        code="$(curl --silent --max-time 1 -o /dev/null -w '%{http_code}' \
                    "http://127.0.0.1:$port/" 2>/dev/null || echo 000)"
        if [[ "$code" == "200" ]]; then
            return 0
        fi
        sleep 0.5
    done
    echo "fatal: server ready timeout after ${SERVER_READY_TIMEOUT_SEC}s on port $port" >&2
    cat "$WORK/server.stderr" >&2 || true
    return 1
}

# stop_server は SERVER_PID を kill し、終了完了を wait する。
# 既停止・空 PID のいずれでも安全に動作する（要件 6.7）。
stop_server() {
    if [[ -z "${SERVER_PID:-}" ]]; then
        return 0
    fi
    if kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    SERVER_PID=""
}

# --------------------------------------------------------------------------
# serve 検証ブロック（タスク 5.2 / 5.3、要件 4.2-4.6, 4.10, 4.11, 6.5, 6.8）
# --------------------------------------------------------------------------

# block_serve は SPA / 読み取り API 5 種 / 書き込み禁止モードの 403 を直列検証する。
# `dot` 不在時はブロック全体をスキップする（要件 6.8）。
block_serve() {
    if should_skip_block_serve; then
        record_skip "4.2-4.6,4.10,4.11" "dot command not found; serve block skipped"
        return 0
    fi

    # ---- タスク 5.2: 読み取り系 API ----
    local port_rw
    port_rw="$(find_free_port)"
    if ! start_server "$port_rw" "$SAMPLE_FULL"; then
        FAIL_COUNT=$(( FAIL_COUNT + 1 ))
        echo "[FAIL] req=4.1 server failed to start (read-write mode)" >&2
        return 1
    fi

    _check_get_endpoint "4.2"  "GET /"                  "$port_rw" "/"                          "200" "text/html"        ""
    _check_get_endpoint "4.3"  "GET /api/schema"        "$port_rw" "/api/schema"                "200" "application/json" "users"
    _check_get_endpoint "4.4"  "GET /api/layout"        "$port_rw" "/api/layout"                "200" "application/json" ""
    _check_get_endpoint "4.5"  "GET /api/export/ddl pg" "$port_rw" "/api/export/ddl?dialect=pg"      "200" ""                "CREATE TABLE"
    _check_get_endpoint "4.6"  "GET /api/export/ddl s3" "$port_rw" "/api/export/ddl?dialect=sqlite3" "200" ""                "CREATE TABLE"

    stop_server

    # ---- タスク 5.3: 書き込み禁止モード ----
    local port_ro
    port_ro="$(find_free_port)"
    if ! start_server "$port_ro" "$SAMPLE_FULL" --no-write; then
        FAIL_COUNT=$(( FAIL_COUNT + 1 ))
        echo "[FAIL] req=4.10 server failed to start (--no-write mode)" >&2
        return 1
    fi

    local schema_status
    schema_status="$(curl -X PUT --silent --max-time 5 -o /dev/null -w '%{http_code}' \
        -H 'Content-Type: text/plain' \
        --data-binary @"$SAMPLE_FULL" \
        "http://127.0.0.1:$port_ro/api/schema" 2>/dev/null || echo 000)"
    assert_status "4.10" "PUT /api/schema in --no-write" 403 "$schema_status"

    local layout_status
    layout_status="$(curl -X PUT --silent --max-time 5 -o /dev/null -w '%{http_code}' \
        -H 'Content-Type: application/json' \
        --data '{"nodes":{}}' \
        "http://127.0.0.1:$port_ro/api/layout" 2>/dev/null || echo 000)"
    assert_status "4.11" "PUT /api/layout in --no-write" 403 "$layout_status"

    stop_server
}

# _check_get_endpoint は GET エンドポイントの応答ステータス・Content-Type・本文ニードルを検証する。
# expected_ctype が空文字なら Content-Type 検査をスキップ、needle が空文字なら本文検査をスキップする。
_check_get_endpoint() {
    local req="$1" label="$2" port="$3" path="$4" expected_status="$5" expected_ctype="$6" needle="$7"
    local body_path="$WORK/curl_body.$req"
    local meta
    meta="$(curl --silent --max-time 5 -o "$body_path" -w '%{http_code}|%{content_type}' \
        "http://127.0.0.1:$port$path" 2>/dev/null || echo '000|')"
    local status="${meta%%|*}"
    local ctype="${meta#*|}"
    assert_status "$req" "$label status" "$expected_status" "$status"
    if [[ -n "$expected_ctype" ]]; then
        assert_contains "$req" "$label content-type" "$ctype" "$expected_ctype"
    fi
    if [[ -n "$needle" && -f "$body_path" ]]; then
        assert_contains "$req" "$label body needle" "$(cat "$body_path")" "$needle"
    fi
}

# --------------------------------------------------------------------------
# 異常系検証ブロック（タスク 6 / 要件 2.10, 3.6, 5.1-5.6）
# --------------------------------------------------------------------------

# block_errors は erdm の異常系終了コードと標準エラーメッセージを 8 ケース検証する。
# 期待される失敗を扱うため、終了コード捕捉時はサブシェルで `|| true` を付け、
# `set -e` 下でもスクリプト全体を停止させない（要件 6.6 / design 判断 5）。
block_errors() {
    local nonexistent="$WORK/nonexistent.erdm"
    local invalid="$WORK/invalid.erdm"
    printf '# Title: bad\nusers\n    +id [\n' >"$invalid"
    mkdir -p "$WORK/empty_path"

    # 5.1-5.3 は dot 不在環境でも検証可能とするため `--format=elk` で実行する。
    # 既定 dot モードは renderDOT 内で `dot` コマンド検出を最初に行うため、
    # `dot` 不在時は入力ファイル系のエラーに到達する前に短絡してしまう（要件 2.10）。
    # ELK モードは `dot` 検出を行わず requireFile / parser.Parse へ直接到達する。
    _expect_error "5.1" "missing input file" \
        "" "input file:" \
        "$ERDM_BIN" "--format=elk" "$nonexistent"

    _expect_error "5.2" "directory as input" \
        "" "is a directory" \
        "$ERDM_BIN" "--format=elk" "$WORK"

    _expect_error "5.3" "syntactically invalid .erdm" \
        "" "parse" \
        "$ERDM_BIN" "--format=elk" "$invalid"

    _expect_error "5.4" "unknown format" \
        "" "unknown format:" \
        "$ERDM_BIN" "--format=invalid" "$SAMPLE_BASIC"

    _expect_error "5.5" "no args (render usage)" \
        "" "Usage: erdm" \
        "$ERDM_BIN"

    _expect_error "5.6" "serve no args (serve usage)" \
        "" "Usage: erdm serve" \
        "$ERDM_BIN" "serve"

    # 要件 2.10: PATH 制限で dot 不在の render 実行で stderr に該当メッセージが含まれる。
    _expect_error_in_path "2.10" "dot not found in PATH (render)" \
        "" "dot command not found in PATH" \
        "$WORK/empty_path" \
        "$ERDM_BIN" "$SAMPLE_BASIC"

    # 要件 3.6: 構文不正 .erdm を ELK 形式で実行したとき stderr に parse メッセージ。
    _expect_error "3.6" "syntactically invalid .erdm (elk mode)" \
        "" "parse" \
        "$ERDM_BIN" "--format=elk" "$invalid"
}

# _expect_error は与えたコマンドを実行し、終了コードが非ゼロ（>0）であること、および
# stderr に needle が含まれることを検証する。期待値が `expected_exit` で指定されたら
# その値で完全一致検証する（空文字なら「非ゼロであること」のみ検証）。
_expect_error() {
    local req="$1" label="$2" expected_exit="$3" stderr_needle="$4"
    shift 4
    local stderr_path="$WORK/stderr.$req"
    local exit_code=0
    "$@" >/dev/null 2>"$stderr_path" || exit_code=$?
    _assert_nonzero_or_match "$req" "$label" "$expected_exit" "$exit_code"
    assert_contains "$req" "$label stderr" "$(cat "$stderr_path" 2>/dev/null || true)" "$stderr_needle"
}

# _expect_error_in_path は PATH を制限して `_expect_error` 同等の検証を行う。
_expect_error_in_path() {
    local req="$1" label="$2" expected_exit="$3" stderr_needle="$4" path_dir="$5"
    shift 5
    local stderr_path="$WORK/stderr.$req"
    local exit_code=0
    PATH="$path_dir" "$@" >/dev/null 2>"$stderr_path" || exit_code=$?
    _assert_nonzero_or_match "$req" "$label" "$expected_exit" "$exit_code"
    assert_contains "$req" "$label stderr" "$(cat "$stderr_path" 2>/dev/null || true)" "$stderr_needle"
}

# _assert_nonzero_or_match は expected が空なら「終了コード != 0」、
# 値ありなら「expected と一致」を assert_eq / assert_exit_code で検証する。
_assert_nonzero_or_match() {
    local req="$1" label="$2" expected="$3" actual="$4"
    if [[ -n "$expected" ]]; then
        assert_exit_code "$req" "$label" "$expected" "$actual"
        return
    fi
    if [[ "$actual" != "0" ]]; then
        PASS_COUNT=$(( PASS_COUNT + 1 ))
        return
    fi
    echo "[FAIL] req=$req $label: expected nonzero exit, actual exit=$actual" >&2
    FAIL_COUNT=$(( FAIL_COUNT + 1 ))
}

# --------------------------------------------------------------------------
# メインフロー（タスク 7 / 要件 6.2, 6.6, 6.8）
# --------------------------------------------------------------------------

# main は前提条件チェック → 一時ディレクトリ準備 → 環境検出 → 各検証ブロック実行 →
# 失敗カウント集約 → 終了コード決定 の順で全体フローを構成する（design.md §C1）。
# block_* 呼び出しに `|| true` を付与する理由:
#   ブロック内 assert_* が `return 1` を返したとき、`set -e` 下では関数全体が
#   即停止し、後続検証が走らない。要件 6.6 は「1 件でも失敗があれば失敗詳細を
#   標準エラーへ出力し非ゼロ終了する」と定めるが、これは「最初の失敗で停止する」
#   ことを禁止するわけではないものの、design.md は「全アサートを実行して
#   FAIL_COUNT を集約する」ことを採用している（タスク 2.4 の AC 末尾）。よって
#   スクリプト全体としては FAIL_COUNT > 0 を非ゼロ終了の条件とし、ブロック単位の
#   早期終了を吸収する。
main() {
    # detect_environment が has_* を確定させ、check_prerequisites がそれを参照する
    # 依存関係のため detect_environment → check_prerequisites の順で呼ぶ。
    detect_environment
    check_prerequisites
    setup_workdir

    # render(dot): dot 不在時はブロック全体スキップ（要件 6.8）
    if should_skip_block_dot; then
        record_skip "2.1-2.9" "dot command not found; render(dot) block skipped"
    else
        block_render_dot || true
    fi

    # render(elk): dot 非依存（要件 3.5 / 6.8）
    block_render_elk || true

    # serve: dot 不在時はブロック全体スキップ（要件 6.8 / design 判断 7）
    if should_skip_block_serve; then
        record_skip "4.2-4.6,4.10,4.11" "dot command not found; serve block skipped"
    else
        block_serve || true
    fi

    # 異常系: dot 非依存
    block_errors || true

    # 検証結果サマリを stdout へ出力（要件 6.6 派生）
    printf '\n=== Summary: PASS=%d FAIL=%d SKIP=%d ===\n' \
        "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT"

    # 終了コード決定（要件 6.2 / 6.6）
    if (( FAIL_COUNT > 0 )); then
        exit 1
    fi
    exit 0
}

main "$@"
