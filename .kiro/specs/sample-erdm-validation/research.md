# research: sample-erdm-validation

## 1. 既存資産の発見

### 1.1 CLI / 内部パッケージ

| 領域 | ファイル | 提供する機能 | 本feature での扱い |
|------|---------|------------|------------------|
| CLI エントリ | `erdm.go` | `runRender` / `runServe` の振り分け、`requireFile` / `requireDir` 検査、`renderDOT` 5 出力、`renderELK` stdout/file 出力 | 変更なし |
| パーサ | `internal/parser/{parser.peg,parser.peg.go,parser.go,builder.go,errors.go,convert.go}` | `Parse([]byte) (*model.Schema, *ParseError)` | 変更なし |
| ドメインモデル | `internal/model/{schema.go,table.go,column.go,fk.go,index.go,group.go}` | `Schema`/`Table`/`Column`/`FK`/`Index`/`Group` 型と `Schema.Validate()` | 変更なし |
| DOT レンダラ | `internal/dot/{dot.go,view.go}` + `templates/` | Graphviz `dot` 用 DOT を生成 | 変更なし |
| HTML レンダラ | `internal/html/` + `templates/` | Graphviz レンダ済み PNG を埋め込んだ HTML を生成 | 変更なし |
| DDL レンダラ | `internal/ddl/{ddl.go}` + `templates/{pg_ddl.tmpl,sqlite3_ddl.tmpl}` | `RenderPG` / `RenderSQLite` で DDL バイト列を返す | 変更なし |
| ELK レンダラ | `internal/elk/{elk.go,build.go,types.go}` | `Render(*Schema) ([]byte, error)`（ELK JSON） | 変更なし |
| HTTP サーバ | `internal/server/{server.go,schema.go,layout.go,export.go,spa.go,errors.go}` | `Config`/`New`/`Run`、`/api/{schema,layout,export/{ddl,svg,png}}` ハンドラ、`--no-write` 403、原子的 rename | 変更なし |
| 座標ストア | `internal/layout/` | `Load(path)` / `Save(path, l)` | 変更なし |
| シリアライザ | `internal/serializer/` | スキーマ DSL シリアライズ | 変更なし |
| SPA 同梱 | `frontend/dist/{index.html,assets/}` | Vite ビルド成果物（`//go:embed` で同梱） | 変更なし |
| 既存サンプル | `doc/sample/{test,test_jp,test_no_logical_name,test_large_data_jp}.erdm` | DSL 互換性検証用 | 変更なし（要件 7.1） |
| 既存テスト | `cmd_test.go` / `cmd_compat_test.go` / `internal/**/*_test.go` | CLI / API の単体・統合テスト | 変更なし（要件 7.3） |
| 既存スクリプト | `scripts/check-requirements-coverage.sh` | bash + `awk` で要件 ID 網羅性検証（参照モデル） | 変更なし（参考実装） |

### 1.2 PEG 文法と DSL 構文網羅

`internal/parser/parser.peg` を確認した結果、本feature の要件 1.5〜1.13 で求められる DSL 構文はすべて受理可能。

| 要件 | DSL 構文 | PEG ルール |
|------|---------|----------|
| 1.5 | `# Title: ...` | `title_info <- '#' space* 'Title:' space* <title> ...` |
| 1.6 | `+name` | `pkey <- '+' / '*'` + `column_attribute` |
| 1.7 `[NN]` | NOT NULL | `'[' notnull ']'` / `notnull <- 'NN'` |
| 1.7 `[U]` | UNIQUE | `'[' unique ']'` / `unique <- 'U'` |
| 1.7 `[=default]` | DEFAULT | `'[=' <default> ']'` |
| 1.7 `[-erd]` | ERD 非表示 | `'[' <erd> ']'` / `erd <- '-erd'` |
| 1.8 | `0..*--1` 系 | `cardinality <- [01*](..[01*])?` |
| 1.9 | 複合 index | `index_info` の `(col, col)` 部 |
| 1.10 | unique index | `(space+ 'unique')?` |
| 1.11 | `@groups[...]` | `groups_decl` |
| 1.12 | 列コメント `# ...` | `column_comment` |
| 1.13 | 論理名なし `name [type]` | `column_name` 部はオプショナル `( '/' <column_name> ... )?` |

### 1.3 既存サンプルの参考点

`doc/sample/test_large_data_jp.erdm`（`shopping site` 想定の 12 テーブル構成）は構文網羅性が高く、`validation_full.erdm` 設計時に：
- index 単一列・複合・unique の 3 種を含む
- `[NN]` / `[U]` / `[=default]` / `[-erd]` を網羅
- 列コメント `# sha1 でハッシュ化して登録` の例
- 多種カーディナリティ `0..*--1` / `0..*--0..1`

ただし `@groups[...]` と `[=default]` を伴う `+name` 等の組合せは含まないため、`validation_full.erdm` ではこれらを補強する必要がある。

## 2. ギャップ調査の結果

### 2.1 Constraint: dialect 名称の不一致

| 項目 | 要件文（修正前） | 実装値 | 解決策 |
|------|---------------|-------|------|
| 4.5 | `dialect=postgres` | `pg` | **要件文を `dialect=pg` に修正**（本research 実施前に確定）。実装変更は要件 7.2 違反となるため不採用 |
| 4.6 | `dialect=sqlite` | `sqlite3` | **要件文を `dialect=sqlite3` に修正**（同上） |

実装値の根拠（`internal/server/export.go:67-76`）:

```go
switch dialect {
case "pg":
    data, renderErr = ddl.RenderPG(schema)
case "sqlite3":
    data, renderErr = ddl.RenderSQLite(schema)
default:
    writeJSONError(w, http.StatusBadRequest, "invalid_dialect", ...)
}
```

既存テスト（`internal/server/export_test.go:42-90`）も `dialect=sqlite3` 前提で書かれており、後方互換確保の観点でも要件文側の修正が妥当。

### 2.2 Unknown → Resolved: 要件 4.3 の解釈

実装（`internal/server/schema.go:32-49`）は `body, _ := os.ReadFile(SchemaPath)` した後 `parser.Parse(body)` で得た `*model.Schema` を `json.Encoder` で書き出す。**生 DSL バイト列ではなく、JSON エンコード後の構造化レスポンス**を返す。

ただし、`*model.Schema` には `Title` / `Tables[].Name` / `Columns[].Name` / `Columns[].Type` 等の DSL 由来文字列が含まれるため、`validation_full.erdm` 内のテーブル名（例: `users`）は JSON 文字列値として確実に含まれる。要件 4.3 を「JSON 本文に DSL 由来テーブル名を含むこと」と確定した（`requirements.md` 4.3 も同表現に修正済み）。

### 2.3 検証スクリプト周辺の調査

#### 2.3.1 ポート確保方式

| 候補 | メリット | デメリット | 採否 |
|------|--------|----------|------|
| `python3 -c "..."`（`socket.bind(0)`） | 簡潔・確実 | python3 が CI / 開発環境で常時利用可能とは限らない | ❌ 採用しない（CI イメージ `cimg/go:1.26` の python3 同梱が不確実） |
| 固定ポート + リトライ（`bash` + `(echo > /dev/tcp/...)`） | 依存最小 | 競合時のリトライが必要 | ✅ **採用**（`18080` 起点で 50 回まで増分試行） |
| Go ヘルパ実行（`go run scripts/_findport/main.go`） | 確実 | Go ビルド前提と実行コスト | ❌ 採用しない |

`bash` の `</dev/tcp/127.0.0.1/PORT` で接続可否（埋まり判定）を即時に検知できる。Go の `cimg/go:1.26` イメージは bash 標準のため確実に動作する。

#### 2.3.2 `pg.sql` / `sqlite3.sql` の構文検証

| ツール | 確認方法 | 不在時の扱い |
|------|--------|------------|
| `psql` | `psql --set ON_ERROR_STOP=on -d "postgresql://localhost..." -f <file>` 等の DB 接続が必要 / または `pgx` の lex 検証 | **DB 接続を要求するためスキップ可とする**（要件 6.8 と整合）。代替として `psql -f - --no-psqlrc --set ON_ERROR_STOP=on </dev/null` で構文だけ検査する案は接続エラーで止まるため不採用 |
| `sqlite3` | `sqlite3 :memory: ".read /tmp/.../validation_full.sqlite3.sql"` 終了コード | バイナリ不在時はスキップ |

要件 2.8 / 2.9 は「`等`」と例示形式で書かれているため、`psql` 不在時のスキップは要件の意図に反しない。スクリプトは「ツール検出 → 検証/スキップ」を明示ログ出力する。

#### 2.3.3 `dot` 検出方式

`command -v dot >/dev/null 2>&1` で十分（要件 2.10 / 6.8）。本実装は `exec.LookPath("dot")` を使うが、bash の `command -v` は等価な PATH 探索を行う。

#### 2.3.4 一時ディレクトリのクリーンアップ

`mktemp -d -t erdm_validate.XXXXXX` で作成、`trap 'rm -rf "$WORK"; kill $SERVER_PID 2>/dev/null || true' EXIT INT TERM` でクリーンアップ。`set -euo pipefail` と組み合わせて途中失敗時もリーク防止（要件 6.7）。

#### 2.3.5 SPA `frontend/dist` 不在時の対応

`erdm serve` は起動時に `validateSPAIndex` で `frontend/dist/index.html` を要求する（`server.go:155`）。本feature では `frontend/dist/index.html` の存在をスクリプト冒頭の `EnvDetect` で `has_spa_dist` 変数に格納し、**不在時は fatal で `make frontend` を促すメッセージを stderr に出して非ゼロ終了する** 方針を確定（前回 validate-design レビュー §問題 3 への対応）。serve ブロックのみのスキップではなく fatal とした理由:

- `erdm serve` は `validateSPAIndex` 失敗で起動できないため、`block_serve` を「ベストエフォートでスキップ」しても `block_render_dot` 完了後の状態と区別がつかず、ユーザビリティを損なう
- 要件 6.2 は「`dot` 在時 終了コード 0」を要求するため、SPA 不在は明確な前提条件違反として早期検出すべき
- 利用者には `make frontend` を実行する選択肢が常にあり、fatal メッセージで自己解決可能

## 3. テスト戦略の調査

### 3.1 既存パターン

`scripts/check-requirements-coverage.sh` は `set -euo pipefail` + `awk` 解析 + 明示エラー出力 + 終了コード分岐で構成されており、本feature の `scripts/validate_sample.sh` も同じパターンを踏襲する。

### 3.2 `make test` 統合の検討結論

**統合しない**。要件 7.3 は `make test` のグリーン維持を求めるのみで、検証スクリプトの実行を強制しない。スクリプトは別系統（手動 / CI 別ジョブ）で実行する想定。

理由:
- `make test` の実行時間に HTTP サーバの起動/停止やランタイム検証時間（数秒〜10秒）を加えると、開発フィードバックが悪化する。
- スクリプトは `dot` / `psql` / `sqlite3` の有無で挙動が変わるため、`go test` の純粋単体テスト責務とは性質が異なる。
- 既存 `cmd_test.go` / `internal/server/integration_test.go` が同等観点をすでにカバーしているため、CI 時に二重検証する必要がない。

## 4. 採用しない代替案

### 4.1 検証スクリプトを Go テスト化（`scripts/validate_sample_test.go`）

| 観点 | 結論 |
|------|------|
| メリット | `make test` で自動実行される。Go テストと同じ表現力を持つ |
| デメリット | 要件 6.1「`scripts/validate_sample.sh` を提供」と明示的に矛盾。`dot`/`psql` 等の外部ツール検出を Go テストで扱うとスキップ条件管理が複雑化 |
| 結論 | ❌ 不採用。要件文が bash スクリプト名を指定しているため |

### 4.2 dialect エイリアス追加（実装側修正）

| 観点 | 結論 |
|------|------|
| メリット | 要件文の文言を維持可能（`postgres` / `sqlite`） |
| デメリット | 要件 7.2「公開 API シグネチャ不変」「`internal/server` の挙動変更禁止」に抵触する解釈の余地。既存テスト（`export_test.go`）も改修対象になる |
| 結論 | ❌ 不採用。要件文側の修正で同等効果を得られる |

## 5. 参考実装・外部リソース

- `scripts/check-requirements-coverage.sh`: 本feature のスクリプト構造の参考
- `cmd_test.go` / `cmd_compat_test.go`: render モードの観測点（要件 2.x / 3.x / 5.x の検証観点と同等）
- `internal/server/integration_test.go` / `error_response_test.go`: serve モードの観測点（要件 4.x の検証観点と同等）
- `internal/parser/parser_test.go`: パーサ受理範囲の確認（要件 1.x のサンプル設計時の参考）

## 6. 要件 6.8 のスコープ確定

要件 6.8 は「`dot` 不在時は要件 3.x（ELK）と要件 5.x（異常系）のみを検証して終了コード 0」と排他的に規定するため、`block_serve`（要件 4.x、dot に依存しないものも含む）も `dot` 不在時はスキップする方針を確定した（前回 validate-design レビュー §問題 1 への対応）。代替案として「dot に依存しない 4.2-4.6 / 4.10-4.11 は実行する」も検討したが、

- 要件 6.8 の文言が排他的（「のみ」）であり、要件文の解釈を曖昧にしない
- `block_serve` は SPA 同梱検出 / ポート確保 / サーバ起動と起動コストが大きく、`dot` 不在環境では恩恵が小さい
- 要件 4.x の同等観点は既存 Go 統合テスト（`internal/server/integration_test.go`）が常時カバーする

の 3 点から、要件文に厳密準拠する方針を選択した。

## 7. 本feature 対象外要件の整理

要件 6.5 が serve 検証を 4.2-4.6 / 4.10-4.11 に明示限定するため、以下は本feature の検証スクリプトの対象外であり、既存 Go 統合テストが同等観点を担保する（前回 validate-design レビュー §問題 2 への対応）。

| 要件 | 担保する既存テスト |
|------|----------------|
| 4.7（SVG 200） | `internal/server/export_test.go` |
| 4.8（PNG 200） | `internal/server/export_test.go` |
| 4.9（dot 不在 SVG → 503） | `internal/server/export_test.go` / `error_response_test.go` |
| 4.12（PUT /api/schema 正常系 200 + 更新） | `internal/server/schema_test.go` |
| 4.13（PUT /api/schema 不正 → 400 + 未変更） | `internal/server/schema_test.go` / `error_response_test.go` |

これにより本feature の検証カバレッジは要件 6.5 のスコープと完全一致する。`design.md §要件トレーサビリティ §本feature の対象外` にこの判断を表形式で記録した。

## 8. 残課題（実装フェーズで対応）

- `validation_full.erdm` の最終構造（テーブル名・列名・グループ名）は次フェーズ（generate-tasks → implement）で確定する。本researchではDSL構文網羅の必須要件 1.5〜1.13 を満たすという方針のみ確認している。
- 検証スクリプトの行数が 200 行を超える兆候があれば、`scripts/lib/validate_sample/` 配下にブロック単位で分割することを実装フェーズで検討する。
