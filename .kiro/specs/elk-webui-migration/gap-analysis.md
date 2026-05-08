# ギャップ分析: ELK + Web UI 移行 (elk-webui-migration)

> 対応要件: `.kiro/specs/elk-webui-migration/requirements.md`
> 既存コード基準: `erdm.go` / `erdm.peg(.go)` / `templates/` / `templates_files.go` （Go モジュール `github.com/unok/erdm`、Go 1.26.1）

## サマリー

- **スコープ**: 8 フェーズの段階改修（DOT 改善 → DSL 拡張 → パッケージ分割 → ELK JSON → `erdm serve` → 座標保存 → `.erdm` 編集 → エクスポート/Doc）。後半フェーズは新規領域（HTTP API、React+ELK SPA、双方向シリアライズ）が大半を占める。
- **既存資産**: 単一 `erdm.go`（436 行）+ PEG パーサ + `text/template` テンプレートが揃っており、Phase 1〜3 の素地として再利用可能。`templates_files.go` で既に `embed.FS` を採用しており、SPA 同梱方式と整合する。
- **主要ギャップ**: ① `@groups[...]` 構文と内部モデル拡張、② `internal/{model,parser,dot,elk,layout,server}` 分割と `erdm.go` の責務縮小、③ `frontend/`（Vite + React + React Flow + elkjs）と CI でのフロントビルド統合、④ `.erdm` 双方向シリアライズの冪等性確保。
- **主要リスク**: フロント新規構築 + 双方向編集 + HTTP 並行書込み制御の組み合わせ。Graphviz への外部依存（Phase 5 SVG/PNG 出力で dot コマンドが必須）と、PEG → 内部モデル ⇄ `.erdm` テキストの往復冪等性。
- **推奨**: 実装アプローチ **C（ハイブリッド）**。`erdm.go` を薄い CLI ディスパッチャに縮退させ、ドメインロジックは `internal/` に切り出し、SPA は独立 `frontend/` で開発し `embed.FS` で同梱する。これは要件 3.1〜3.6 と直接整合する。

## 要件-資産マッピング

| 要件 | 既存資産 / 関連箇所 | ギャップ種別 | 概要 |
|---|---|---|---|
| 1.1〜1.5 DOT 既定属性 | `templates/dot.tmpl`（`digraph g { node [...] }` のみ） | Missing | `rankdir=LR` / `splines=ortho` / `nodesep=0.8` / `ranksep=1.2` / `concentrate=false` をテンプレートに追加するだけ。低リスク。 |
| 1.6〜1.7 親→子方向への反転 | `templates/dot_relations.tmpl`（`{{$t.TitleReal}} -> {{.Relation.TableNameReal}}`、つまり**子→親**を出している） | Missing | 方向反転とエッジ重複統合の不実施。属性 `arrowhead/headlabel/taillabel` 側も親→子に揃えて見直し必要。 |
| 1.8 `-erd` カラムの除外 | `dot_relations.tmpl` で `(not .WithoutErd)` 適用済み、`Column.WithoutErd` も保持済み | OK（再利用可） | 既存ロジックを引き継ぐ。 |
| 1.9 スナップショット再生成 | `test/.empty`（実テスト未整備） | Missing | テストハーネス自体を新設する必要がある（Go の `testing` + ゴールデンファイル想定）。 |
| 2.1〜2.9 `@groups[...]` 構文 | `erdm.peg`（`table_name_info` 直下）、`erdm.peg.go`（自動生成） | Missing | PEG ルール追加 + アクションで `model.Table` に `Groups []string` を保持。`peg` ツールで再生成必要。 |
| 2.10〜2.12 cluster 描画 | DOT テンプレート群（cluster 概念なし） | Missing | `subgraph cluster_<group>` 生成ロジックを `internal/dot` に追加。primary group 単独所属、ungrouped はトップレベル。 |
| 3.1〜3.4 `internal/` 分割 | `erdm.go` 単一集約（型・パーサ駆動・テンプレート I/O・`dot` 起動・ファイル I/O が同居） | Missing | `internal/model` `internal/parser` `internal/dot` `internal/elk` `internal/layout` `internal/server` を新規作成し、`erdm.go` は CLI ディスパッチャへ縮退。 |
| 3.5〜3.6 後方互換 | 現 CLI は `erdm [-output_dir DIR] schema.erdm` | Constraint | 既存出力ファイル名・拡張子（`.dot`/`.png`/`.html`/`.pg.sql`/`.sqlite3.sql`）を維持。差分は要件 1 由来の DOT 属性のみ許容。 |
| 4.1〜4.7 ELK JSON | 既存ロジックなし | Missing | `internal/elk` を新設。`elkjs` 入力スキーマ準拠（`id/width/height/sources/targets/children/properties` 構造）。`--format=elk` を `flag` に追加。 |
| 5.1〜5.12 `erdm serve` | 既存ロジックなし。`flag` のみでサブコマンド体系なし | Missing | `os.Args` 解析で `serve` を分岐するか、`flag.NewFlagSet` を使う。`net/http` + `embed.FS` ベース。`embed.FS` 採用は前例あり（`templates_files.go`）で一致する。 |
| 6.1〜6.6 レイアウト保存 | 既存ロジックなし | Missing | `<schema>.erdm.layout.json` を `internal/layout` で読み書き。`--no-write` モードのフラグ評価と並行制御（10.2）が要件で連動。 |
| 7.1〜7.10 `.erdm` 編集 + 往復冪等性 | パーサ単方向のみ（`.erdm` → `ErdM`）。シリアライザ未実装 | Missing | 内部モデル → `.erdm` テキスト変換 + `localStorage` 連携 SPA + パースエラー時の元ファイル保護（10.3）。要往復テスト。 |
| 8.1〜8.6 エクスポート UI/Doc | DDL テンプレートあり（pg/sqlite3）。SVG/PNG は現状 dot 経由で PNG のみ。 | Missing | DDL ダウンロード API は既存テンプレ流用。SVG 生成は `dot -Tsvg` の追加実装。`README.md` 追記必要。 |
| 9.1〜9.6 互換性・配布・テスト | `embed.FS` 採用済（templates）、Go 単一バイナリ、ビルドは `build.sh`（`peg` + `gox`） | Constraint | フロント `dist/` を `embed.FS` 配下に追加。CI（`.circleci/`）はビルド工程に Vite ビルドを追加する必要あり。`go-bindata` は実コードでは未使用（`templates_files.go` は embed 化済み）→ `build.sh` は古い記述で要更新。 |
| 9.4 `dot` 不在時のフォールバック | 現状 `main` 冒頭で `dot -?` 失敗時は即 return | Constraint | `erdm serve` では起動可、SVG/PNG エクスポートのみ HTTP 503 を返す挙動に変更必要（CLI 側の挙動とは別系統で扱う）。 |
| 10.1〜10.4 安全性 | 現状はファイル読み取りエラー時に return のみ。`os.Rename` ベースの原子的書き込み未実装。signal handling なし。 | Missing | プロセス内ロック（`sync.Mutex`）、`os.CreateTemp` + `os.Rename`、`http.Server.Shutdown` での graceful 終了。 |

### 既存制約・前提

- **Graphviz `dot` 依存**: 現 CLI は `dot -?` を `main` 冒頭で必ず実行。要件 9.4 と整合させるため、`erdm serve` 起動時は dot 不在を許容しつつ SVG/PNG API のみ 503 化する分岐が必要（CLI 既定モードの dot 必須は維持）。
- **Go バージョン**: `go 1.26.1`。`embed`、`os.Root` 等の新 API 利用可。
- **PEG ツールチェイン**: `build.sh` は `peg` バイナリで `erdm.peg.go` を再生成する前提。`@groups` 追加時は同ツールが必要（CI へ組み込み要）。
- **テスト基盤の不在**: `test/` ディレクトリは空。Go 標準 `testing` ベースのスナップショット/ゴールデンファイル基盤を新設する必要がある（要件 1.9 / 9.5 / 9.6）。

### 要調査（設計フェーズで解消）

- `peg` ツール（`pointlander/peg` 系）のバージョン固定方法と CI への組込み手順。
- React Flow / elkjs の最新版で「ELK 自動配置 → 個別ノードのドラッグで `<schema>.erdm.layout.json` 反映」を成立させる API。
- フロント `dist` を `go:embed` するときのファイル監視・キャッシュ無効化（開発時の DX）。
- `.erdm` シリアライザの正規化規則（カラム順、`@groups` の引用、コメント保持の有無）—往復冪等性 7.10 の判定基準。
- Graphviz 不在環境での「画像なし HTML 出力」の互換性（既存 `html.tmpl` は `<img src="...">` 前提）。

## 実装アプローチ

| 案 | 概要 | 適合度 | メリット | デメリット |
|---|---|---|---|---|
| **A: 既存拡張** | `erdm.go` 単一パッケージで全機能を実装 | 低 | 移行差分が小、既存 import を維持 | 要件 3.1（`internal/{...}` 分割）と直接矛盾。1 ファイル 1000 行超は確実。テスト容易性が悪化。 |
| **B: 新規作成（フルリプレース）** | `cmd/erdm` 配下で全 CLI を再実装、`erdm.go` は廃止 | 中 | 構造を完全制御可能 | 後方互換 9.1〜9.2 の検証コストが膨らむ。`erdm.peg.go` 自動生成資産との接続再設計が必要。 |
| **C: ハイブリッド（推奨）** | エントリは `erdm.go` を維持して薄い CLI ディスパッチャに縮退、ドメイン処理は `internal/{model,parser,dot,elk,layout,server}` へ移設、フロントは独立 `frontend/` で開発し `dist` を `embed.FS` 同梱 | 高 | 要件 3.1〜3.4 と完全一致、Phase 1〜3 を順次マイグレート可能、`templates_files.go` の `embed.FS` 流儀を踏襲、後方互換 9.1 を `internal/dot` 経路の単体テストで担保 | フェーズ間で `erdm.go` ↔ `internal/` の暫定アダプタが必要。フロントビルドを伴う CI 二段階化。 |

**推奨: C（ハイブリッド）**

判断根拠:
- 要件 3.1〜3.4 が `internal/{model,parser,dot,elk,layout,server}` の存在を必須化しているため、A 案は要件不適合、B 案は過剰スコープ。
- 既存 `templates_files.go` で `embed.FS` を使用しているため、SPA 同梱の流儀（`go:embed frontend/dist`）と整合する。
- Phase 1（DOT 属性）→ Phase 3（パッケージ分割）の順で進めれば、各フェーズで後方互換の単体テスト（既存 DDL/HTML 出力比較）を回しながら段階移行できる。

## 工数・リスク

### 全体

| 指標 | 評価 | 根拠 |
|---|---|---|
| 工数 | **XL（2 週超）** | フロント新規（React+Vite+React Flow+elkjs）+ HTTP API + 双方向シリアライズ + 既存 CLI のリファクタの 4 軸が並走。 |
| リスク | **High** | 双方向シリアライズの冪等性、embed.FS でのフロント同梱と CI ビルド統合、並行書込み制御、Graphviz の有無による分岐挙動と未知技術の組合せが多い。 |

### フェーズ別

| Phase | 工数 | リスク | 根拠 |
|---|---|---|---|
| 1. DOT 属性改善 + 親→子反転 | S（1〜3 日） | Low | テンプレート差分のみ。スナップショット基盤は併設で立てる。 |
| 2. `@groups[...]` DSL 拡張 | M（3〜7 日） | Medium | PEG 再生成と `peg` ツール準備、cluster 描画ロジック追加。Phase 1 と独立に進められる。 |
| 3. `internal/` への分割 | M（3〜7 日） | Medium | `erdm.go` の責務分割。後方互換テスト（既存 5 出力の同等性）が並走する。 |
| 4. ELK JSON 出力 | M（3〜7 日） | Medium | 仕様は明確（elkjs 入力スキーマ）だが、primary/secondary group の階層化を伴う。 |
| 5. `erdm serve` 基盤 | L（1〜2 週） | High | サブコマンド導入、`net/http`、SPA 配信、SVG/PNG エクスポート、frontend 初期化。dot 不在時の 503 分岐含む。 |
| 6. レイアウト保存 | M（3〜7 日） | Medium | `<schema>.erdm.layout.json` の I/O と SPA 連携、`--no-write` モードと並行制御。 |
| 7. `.erdm` 編集機能 | L（1〜2 週） | High | 双方向シリアライズの冪等性、SPA 編集 UI、`localStorage` 下書き、エラー時の元ファイル保護。 |
| 8. エクスポート UI + Doc | S（1〜3 日） | Low | API は既存。SPA から呼ぶ UI と README/Doc 追記のみ。 |

## 設計フェーズへの推奨事項

### 推奨アプローチ

- **C（ハイブリッド）採用**を前提に、`internal/` 配下のパッケージ責務を以下の方針で詰める:
  - `internal/model`: 純粋な構造体（`Schema`, `Table`, `Column`, `FK`, `Index`, `Group`）。`Groups []string`（先頭 = primary）を `Table` に追加。`erdm.go` の現 `Table/Column/Index` 構造から logical name と PK 番号管理ロジックを移送。
  - `internal/parser`: `erdm.peg.go` を所有し、`Parse([]byte) (*model.Schema, error)`。`erdm.go` 側のレシーバメソッド群（`addTableTitleReal` 等）はパーサ内専用に閉じ込め、外向けには `model.Schema` のみ返す。
  - `internal/dot`: テンプレート 3 種（dot/tables/relations）を内包し、属性既定値（要件 1.1〜1.5）と cluster ロジック（要件 2.10〜2.12）を実装。`Render(model.Schema) (string, error)` を公開。
  - `internal/elk`: `model.Schema → ELK JSON` 変換のみ。`elkjs` の `ElkNode` 互換構造体を独自定義し、`encoding/json` でシリアライズ。
  - `internal/layout`: `<schema>.erdm.layout.json` の Load/Save、JSON 破損時のエラー型化（要件 6.6）。
  - `internal/server`: HTTP ハンドラと middleware、書込みロック（要件 10.2）、原子的置換（要件 10.3）、graceful shutdown（要件 10.4）。`embed.FS` フィールドはここから注入する。
- **CLI ディスパッチ**: `erdm.go` の `main` を「サブコマンド有無の判定」だけに縮退させ、`run(args)` と `runServe(args)` に分割。`-output_dir`/`schema.erdm` の従来 CLI 形態を `flag.FlagSet` で互換維持し、`erdm serve` を新サブコマンドとして並列導入。
- **フロントエンド**: `frontend/` を独立ディレクトリで Vite + React + TypeScript で立ち上げ、ビルド成果物 `frontend/dist` を `internal/server` から `//go:embed` で参照。CI は「`peg erdm.peg` → `npm ci && npm run build`（frontend）→ `go build`」の 3 段に整理し `build.sh` を更新する。
- **テスト基盤**: `internal/dot` `internal/elk` `internal/parser` の各パッケージに `_test.go` を新設しゴールデンファイル比較を導入（要件 1.9 / 9.5 / 9.6）。`testdata/` 規約に従う。
- **互換性ガード**: `internal/dot` 出力の旧テンプレート差分は要件 1 由来のみであることを diff テストで明示する（要件 3.6）。

### 要調査項目（設計時に解消必須）

1. `pointlander/peg`（または同等 PEG ジェネレータ）のバージョン固定と CI 取得方法。
2. React Flow + elkjs での「ELK 自動配置 → ドラッグで個別座標上書き → サーバへ PUT」のリファレンス実装。
3. `frontend/dist` の `go:embed` で起動毎に再ビルドさせない CI/ローカル DX のフロー（`go generate` で frontend ビルドをトリガーする是非）。
4. `.erdm` シリアライザの正規化規則（カラム属性順、`@groups` 引用、コメント保持、空行扱い）と往復冪等性のテスト戦略（要件 7.10）。
5. `dot -Tsvg` が利用可能な前提でも、`dot` 不在時に SVG/PNG API のみ 503 化する具体的フォールバック設計（要件 5.7 / 5.8 / 9.4）。
6. 既存 `html.tmpl`（PNG 埋込み HTML）の Web UI 移行後の取扱（廃止 / 並行維持）— `update.md` §6 でも未確定として残されている。

### スコープ留意事項

- 後方互換要件 9.1〜9.2 を破壊しないため、`internal/dot` への移送と `erdm.go` の縮退は**同一フェーズ**で実施し、出力差分テストを必ず添える。
- 要件外の追加（`--focus` 等のサブセット出力、Graphviz 廃止）は本フィーチャーで扱わない（`update.md` §6 の通り後続検討）。
