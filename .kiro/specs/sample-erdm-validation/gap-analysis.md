# gap-analysis: sample-erdm-validation

## サマリー

- `erdm` CLI（render / serve）と `internal/{parser,dot,ddl,html,elk,server,layout,serializer}`、SPA 同梱（`frontend/dist/`）は既に動作可能な状態にあり、本feature の主目的（検証用サンプル `.erdm` と検証スクリプト追加）は CLI 実装の改修なしで成立する見込み。
- ただし、要件 4.5 / 4.6 の `dialect=postgres` / `dialect=sqlite` は **既存実装が受理しない**（実装は `pg` / `sqlite3` のみ受理し、他は 400）。要件側を実装値に合わせるか、サーバ側に別名を追加するかの選択が必要（**最重要ギャップ**）。
- 要件 4.3「`GET /api/schema` のレスポンス本文に DSL 文字列を含む」は、既存実装が `*model.Schema` を JSON エンコードして返す挙動と整合しない可能性が高く、要件文の解釈確定が必要（**Unknownギャップ**）。
- 推奨アプローチは **C: ハイブリッド**。検証用サンプル・検証スクリプトを新規追加（B: 新規）しつつ、上記 dialect 名称ギャップは設計フェーズで「要件を実装に合わせて修正」を第一案として整合させる（A: 既存拡張側の最小修正）。
- 工数 **S（1-3日）** / リスク **Low**。実装変更はゼロ〜極小、追加成果物のみで要件を満たせる構造のため。

## 要件-資産マッピング

### 充足する既存資産

| 要件 | 既存資産 | 状態 |
|------|---------|------|
| 1.3 / 1.4（パース成功） | `internal/parser.Parse`（PEG: `parser.peg`） | OK — 1.5〜1.13 の構文は PEG 文法で全て受理可能（タイトル、`+name`、`[NN]/[U]/[=default]/[-erd]`、`0..*--1`/`1--0..1`/`1--1`、`index ... unique`、`@groups[...]`、`# ...` 列コメント、論理名なし `name [type]`） |
| 2.1〜2.6（DOT モード 5 出力） | `runRender→renderDOT`（`erdm.go:149`）、`internal/{dot,html,ddl}` | OK — 旧 CLI 互換で `<basename>.{dot,png,html,pg.sql,sqlite3.sql}` を生成 |
| 2.7（dot 再描画成功） | `internal/dot.Render` 出力 | OK — Graphviz 互換 DOT を生成 |
| 2.8（pg.sql 構文 OK） | `ddl.RenderPG`（テンプレート `templates/pg_ddl.tmpl`） | OK — `psql` 互換 DDL（DROP TABLE IF EXISTS ... CASCADE, CREATE TABLE, CREATE INDEX）を生成 |
| 2.9（sqlite3 構文 OK） | `ddl.RenderSQLite`（テンプレート `templates/sqlite3_ddl.tmpl`） | OK |
| 2.10（dot 不在時のエラー） | `renderDOT` 冒頭の `exec.LookPath("dot")`（`erdm.go:150`） | OK — `dot command not found in PATH` を含むエラーを返す |
| 3.1〜3.5（ELK モード） | `runRender→renderELK`（`erdm.go:112`）、`internal/elk.Render` | OK — `--format=elk` で `dot` 検査をスキップ、`-output_dir` 明示で `<basename>.elk.json`、未指定で stdout |
| 3.6（ELK パースエラー） | `renderELK` 内 `parser.Parse` エラー時 `parse %s: %w` | OK |
| 4.1（HTTP リッスン） | `runServe→server.New→Run`、`net.JoinHostPort` | OK |
| 4.2（GET / が 200 / text/html） | `server.handleSPA`（`spa.go:14`、`Content-Type: text/html; charset=utf-8`） | OK |
| 4.4（GET /api/layout → 200 / application/json） | `server.handleGetLayout`（`layout.go:30`、`application/json; charset=utf-8`） | OK |
| 4.7 / 4.8（SVG/PNG 200） | `server.handleExportImage`（`export.go:101`） | OK — `image/svg+xml` / `image/png` を返す |
| 4.9（dot 不在時 SVG → 503） | `handleExportImage` の `cfg.HasDot` チェック | OK |
| 4.10 / 4.11（`--no-write` 時 PUT → 403） | `handlePutSchema`（`schema.go:57`）、`handlePutLayout`（`layout.go:48`） | OK |
| 4.12（PUT /api/schema 200 + ファイル更新） | `handlePutSchema` + `writeSchemaAtomic`（原子的 rename） | OK |
| 4.13（不正 DSL → 400 + 未変更） | `handlePutSchema` の `parser.Parse` 失敗時 400 + write 前に return | OK |
| 5.1（不在ファイル → `input file:`） | `requireFile`（`erdm.go:265`） | OK |
| 5.2（ディレクトリ → `is a directory`） | `requireFile` の `IsDir` 分岐 | OK — `is a directory, want a regular file` を出力 |
| 5.3（構文エラー → `parse`） | `renderDOT` / `renderELK` の `parse %s: %w` | OK |
| 5.4（unknown format） | `runRender` default case の `unknown format: %s` | OK |
| 5.5 / 5.6（usage） | `usageRender`「Usage: erdm ...」、`usageServe`「Usage: erdm serve ...」 | OK |
| 7.1（既存サンプル不変） | `doc/sample/test*.erdm` は新規ファイル追加のみで未変更 | OK（運用上の制約として遵守する） |
| 7.2（公開 API 不変） | 本feature は internal パッケージの公開シグネチャ変更を要しない | OK |
| 7.3（`make test` グリーン） | 既存テスト群（`cmd_test.go` / `cmd_compat_test.go` / `internal/**/*_test.go`） | OK（変更なし）／ただし `scripts/check-requirements-coverage.sh` は `.kiro/specs/elk-webui-migration/design.md` を参照しており、本feature 追加で破綻しない |

### Missing（新規作成が必要な成果物）

| 要件 | 必要な追加 |
|------|-----------|
| 1.1 / 1.5〜1.13 | `doc/sample/validation_basic.erdm`、`doc/sample/validation_full.erdm` を新規作成。`validation_full.erdm` は要件 1.5〜1.13 の構文網羅を満たす形で構築する |
| 1.2 | `doc/sample/validation_basic.erdm` を新規作成（最小構成） |
| 6.1〜6.8 | `scripts/validate_sample.sh` を新規作成。要件 2.x / 3.x / 4.x（4.2〜4.6 + 4.10〜4.11） / 5.x の実行を直列に検証し、`dot` 不在時は要件 2.x の dot 必須項目をスキップ、エラー時は非ゼロ終了、一時ディレクトリは `mktemp -d` + `trap` でクリーンアップ |

### Constraint / Mismatch（既存実装と要件文の不一致）

| 要件 | 不一致の内容 | 解決方針候補 |
|------|------------|-------------|
| **4.5 / 4.6** | 要件文は `dialect=postgres` / `dialect=sqlite`。実装（`internal/server/export.go:67-76`）は `pg` / `sqlite3` のみ受理し、他は 400 + `invalid_dialect`。`internal/server/export_test.go:42-90` も `dialect=sqlite3` / `dialect=mysql` を前提にしている | **(a) 要件側を `dialect=pg` / `dialect=sqlite3` に修正**（実装変更ゼロ／既存テストとの整合維持／推奨）／(b) サーバ側に `postgres`→`pg`、`sqlite`→`sqlite3` のエイリアスを追加（要件 7.2 の API 不変制約に該当しないが、テスト追加が必要） |
| **4.3** | 要件文「レスポンス本文に `validation_full.erdm` の DSL 文字列を含めなければならない」。実装（`internal/server/schema.go:32-49`）は `*model.Schema` を `encoding/json` でエンコードした JSON を返し、原文 DSL バイト列はそのまま含めない（テーブル名・列名等は JSON 文字列値として含まれるため、解釈次第で「含む」とも言える） | **要件文の解釈確定が必要**: (a) JSON 中にスキーマ由来文字列（テーブル名・列名・型）が含まれれば良い → 実装変更不要／(b) 原文 DSL バイト列を返す（または含める） → 設計上の API 変更（SPA 影響あり）。設計フェーズで (a) を採用するのが既存仕様と整合 |

### Unknown（設計フェーズで調査・確定するもの）

| 項目 | 概要 |
|------|------|
| `validation_full.erdm` の論理スキーマ | DSL 主要構文を全て使う最小集合（テーブル数 ≧ 3、外部キー 3 種、index 2 種、列属性 4 種、`@groups`、列コメント、論理名なし列）。既存 `test_large_data_jp.erdm` を参考に骨組みを設計するが、別ファイルとして新規にまとめる |
| 検証スクリプトのポート選定 | 一時ポート確保方法（`ss -ltn` 検査 / Python 経由 `socket.bind(0)` / `python3 -c '...'` 等）と CI 環境（`.circleci`）での再現性 |
| 検証スクリプトの実行依存 | `curl` / `jq`（ELK JSON のパース確認に利用するか）/ `python3` 等、CI イメージで利用可能なツール |
| `make test` への組込み有無 | 検証スクリプトを `make test` に組み込むか別 target（例: `make validate-sample`）にするか。要件 7.3 は組込み必須化していないため、別 target が無難 |
| `pg.sql` の構文検証手段 | 要件 2.8 は「pgx / psql --set ON_ERROR_STOP=on -f 等」と例示。CI と開発環境のいずれにも存在する手段（おそらく `psql` 不在 → スキップ）の取り扱い |

## 実装アプローチ

### A: 既存拡張（実装側のみ調整）

- **概要:** 既存 CLI / API の挙動はそのまま、要件文の不一致箇所だけ実装側で吸収する（4.5/4.6 のエイリアス追加、4.3 の DSL バイト返却切替、等）。
- **メリット:** 要件文の文言を維持できる。
- **デメリット:** 要件 7.2「公開 API 不変」「既存サンプル不変」と矛盾する変更が必要になる場面がある（例: `GET /api/schema` のレスポンス形式変更は SPA 側互換に影響）。実装変更がスコープを越える恐れ。
- **適否:** ❌ 単独では避ける。

### B: 新規作成（追加成果物のみで完結）

- **概要:** 検証用サンプル `.erdm` 2 種、検証スクリプト 1 本を新規追加するのみ。CLI / 内部パッケージは触らない。
- **メリット:** 既存実装・既存サンプル・既存テストへの影響ゼロ。要件 7.1 / 7.2 / 7.3 を自然に満たす。最短工数。
- **デメリット:** 要件 4.5 / 4.6 の dialect 名称ギャップを「要件文の修正」で吸収しないと、検証スクリプトが 400 を踏んで要件 6.5 を満たせない。
- **適否:** ✅ 主軸とするが、要件文側の小修正が前提。

### C: ハイブリッド（推奨）

- **概要:** B（追加成果物）を主軸に、要件 4.5 / 4.6 の dialect 名称を実装値（`pg` / `sqlite3`）に合わせて要件側で修正、要件 4.3 の解釈を「JSON 本文中に DSL 由来の文字列を含む」と確定。実装変更は行わない。
- **メリット:** 既存実装・テストを温存しつつ、要件・実装・検証スクリプトを整合させられる。要件 7.2 を完全遵守。
- **デメリット:** 要件文の表現修正という小規模な後戻り工程が発生する（generate-requirements の出力に対する微修正）。
- **適否:** ✅ **採用推奨**。

## 工数・リスク

- **工数: S（1-3日）**
  - 根拠: 追加成果物が `validation_basic.erdm`（≦ 30 行）/ `validation_full.erdm`（≦ 100 行）/ `validate_sample.sh`（≦ 200 行）の 3 ファイルに収束。既存実装の改修は不要（要件文の dialect 名称修正のみ）。サンプル DSL の構文網羅と検証スクリプトのスキップ分岐の検証に半日〜1日。
- **リスク: Low**
  - 根拠: 既知技術（bash + curl + go）、既存パターンの拡張（`scripts/check-requirements-coverage.sh` の構造を参照可能）、明確なスコープ（追加のみ）、CLI / API 側の既存テスト（`cmd_test.go` / `cmd_compat_test.go` / `internal/server/integration_test.go`）が同等観点を既に検証しており、外部からの再検証はその上乗せに留まる。

## 設計フェーズへの推奨事項

### 推奨アプローチ

**C: ハイブリッド**を採用する。

1. **要件文の小修正を design-phase 入力で確定する:**
   - 要件 4.5 / 4.6 の `dialect=postgres` → `dialect=pg`、`dialect=sqlite` → `dialect=sqlite3` に揃える（既存実装値が source of truth）。
   - 要件 4.3 の「DSL 文字列を含めなければならない」を、実装の現状（`*model.Schema` を JSON エンコード）に合わせて「`application/json` を返し、レスポンス本文に DSL 由来のテーブル名/列名等が JSON 文字列として含まれること」と解釈確定する（または要件文を「JSON 形式のスキーマ表現を返す」に整える）。

2. **追加成果物の構造を確定する:**
   - `doc/sample/validation_basic.erdm`: 要件 1.3 用の最小サンプル（`test.erdm` と同等規模で十分）。
   - `doc/sample/validation_full.erdm`: 要件 1.5〜1.13 を 1 ファイルで網羅する構造。`test_large_data_jp.erdm` の構文網羅を参考に、行数 100 行以内で集約。
   - `scripts/validate_sample.sh`: bash + `set -euo pipefail` + `trap` クリーンアップ。検証ブロックを「render(dot)」「render(elk)」「serve」「異常系」に分割し、`dot` 検出有無で render(dot) ブロックをスキップ。

3. **`make test` には組み込まない:**
   - 要件 7.3 は `make test` のグリーン維持を要求しているのみで、検証スクリプトを `make test` 内に含める指示はない。組込みは CI 構成変更を伴うため別タスクに分ける。

### 要調査項目（design.md で確定すべきもの）

1. **検証スクリプトのポート確保方式**: 一時ポートを動的取得する手段（`python3 -c "import socket; s=socket.socket(); s.bind(('127.0.0.1',0)); print(s.getsockname()[1])"` 等）を CI の `.circleci/config.yml` 環境で動作させられるか確認。
2. **`pg.sql` の SQL 構文チェック手段**: `psql` が CI にあるか／なければ `pg_query_go` 等のスキップフォールバック方針を決定。要件 2.8 は「等」と書かれているため、`psql` 不在時は warning 扱いも許容できるが、要件 6.2 / 6.3 の動作仕様として明文化が必要。
3. **`sqlite3 :memory:` 投入手段**: `sqlite3 :memory: ".read /tmp/.../validation_full.sqlite3.sql"` の終了コード判定方式を決定。
4. **要件 4.3 の最終解釈**: 上述のとおり、設計フェーズの最初に確定。実装変更を伴うか否かで以降のタスク量が変わる。
5. **検証スクリプトのスキップ判定基準**: 要件 6.8 の「`dot` コマンドが PATH 上に存在しない場合」は `command -v dot` で十分か、`exec.LookPath` 相当の挙動が必要かを確定。
