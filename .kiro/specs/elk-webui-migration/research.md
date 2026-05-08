# 発見ログ: ELK + Web UI 移行 (elk-webui-migration)

> 対応設計: `.kiro/specs/elk-webui-migration/design.md`
> 既存ギャップ分析: `.kiro/specs/elk-webui-migration/gap-analysis.md`

このドキュメントは設計の前提となる発見プロセス（既存コード観察・統合ポイント特定・外部技術調査・要調査項目への結論）を記録する。

## 1. 既存コードベースの調査

### 1.1 リポジトリ構成

| パス | 種別 | 行数/サイズ | 役割 |
|---|---|---|---|
| `erdm.go` | Go ソース | 436 行 | CLI エントリ + 全ドメイン処理（モデル定義、PEG パーサ駆動、テンプレート出力、`dot` 起動、ファイル I/O） |
| `erdm.peg` | PEG 文法 | 44 行 | `.erdm` の文法定義 |
| `erdm.peg.go` | 自動生成 Go | 約 66 KB | `pointlander/peg` 系で生成された `Parser` 実装 |
| `templates_files.go` | Go ソース | 11 行 | `templates/*.tmpl` を `embed.FS` で同梱 |
| `templates/dot.tmpl` | text/template | 8 行 | `digraph g { ... }` の外殻、`node [shape=box,style=rounded,...]` のみ |
| `templates/dot_tables.tmpl` | text/template | 22 行 | テーブルノード描画（HTML-like label） |
| `templates/dot_relations.tmpl` | text/template | 12 行 | エッジ出力（**現状は子→親方向**） |
| `templates/html.tmpl` | html/template | 約 200 行 | PNG 埋込み + テーブル一覧 + サイドバー検索 |
| `templates/pg_ddl.tmpl` | text/template | 15 行 | PostgreSQL DDL |
| `templates/sqlite3_ddl.tmpl` | text/template | — | SQLite3 DDL |
| `build.sh` | シェル | 7 行 | `peg erdm.peg` → `go-bindata`（**死コード**、現状 `embed.FS`）→ `gox` クロスビルド |
| `test/.empty` | 空 | 0 byte | テスト未整備 |
| `doc/sample/*.erdm` | サンプル | 4 ファイル | `test.erdm` `test_jp.erdm` `test_large_data_jp.erdm` `test_no_logical_name.erdm` |
| `.circleci/` | CI | — | 詳細未確認（フロントビルドの追加が必要） |
| `go.mod` | Go モジュール | — | `module github.com/unok/erdm` / `go 1.26.1` |

### 1.2 既存ドメイン型（`erdm.go` 内）

```text
ErdM
 ├─ Title          string
 ├─ Tables         []Table
 ├─ CurrentTableId int
 ├─ ImageFilename  string
 └─ IsError        bool

Table
 ├─ TitleReal       string  // 物理名
 ├─ Title           string  // 論理名
 ├─ Columns         []Column
 ├─ CurrentColumnId int
 ├─ PrimaryKeys     []int   // PK カラムインデックス
 ├─ Indexes         []Index
 └─ CurrentIndexId  int

Column
 ├─ TitleReal/Title    string
 ├─ Type               string
 ├─ AllowNull          bool
 ├─ IsUnique           bool
 ├─ IsPrimaryKey       bool
 ├─ IsForeignKey       bool
 ├─ Default            string
 ├─ Relation           TableRelation { TableNameReal, CardinalitySource, CardinalityDestination }
 ├─ Comments           []string
 ├─ IndexIndexes       []int
 └─ WithoutErd         bool

Index
 ├─ Title    string
 ├─ Columns  []string
 └─ IsUnique bool
```

**所見**:
- `CurrentTableId` / `CurrentColumnId` / `CurrentIndexId` はパース中の状態保持用であり、ドメインモデルには本来不要。新 `internal/model` から除外する。
- `Group` 概念は現状なし。`Table` に `Groups []string`（先頭 = primary）を追加する必要がある。
- `Relation` は `TableNameReal`（参照先テーブル物理名）と cardinality 文字列のみ。要件 1.6（親→子方向）対応のため、レンダリング時に「自分 = 子、`Relation.TableNameReal` = 親」と解釈し、エッジを `親 -> 子` で出すように `internal/dot` で正規化する。

### 1.3 既存 DOT 出力の問題点

- `templates/dot.tmpl` は `digraph g { node [shape=box,style=rounded,height=0.08,fontname="Noto Sans Mono CJK JP"]; ... }` のみで、**`rankdir`/`splines`/`nodesep`/`ranksep`/`concentrate` のいずれも未指定**（要件 1.1〜1.5 でいずれも明示が必要）。
- `templates/dot_relations.tmpl` の `{{$t.TitleReal}} -> {{.Relation.TableNameReal}}` は **`子 -> 親`** を出している。`headlabel`（矢頭側）に `CardinalitySource`、`taillabel`（矢尾側）に `CardinalityDestination` を当てている点も方向反転に伴って入れ替えが必要（要件 1.6）。
- 同一親子間で複数 FK がある場合に独立エッジで出されている（要件 1.7 を既に満たしている）。

### 1.4 既存 CLI フロー

```
main()
  ├─ exec("dot -?") で Graphviz の存在チェック → 失敗時は即 return
  ├─ flag.Parse() で --output_dir 解釈
  ├─ 入力 .erdm を ioutil.ReadAll
  ├─ Parser{Buffer: ...}.Init/Parse/Execute → ErdM が組み上がる
  ├─ Asset() で 6 テンプレートを読み出し
  ├─ text/template に dot/dot_tables/dot_relations/pg_ddl/sqlite3_ddl を一括登録
  ├─ html/template に html を登録
  ├─ <basename>.dot を出力
  ├─ exec("dot -T png -o <basename>.png <basename>.dot")
  ├─ <basename>.html / <basename>.pg.sql / <basename>.sqlite3.sql を出力
  └─ 終了
```

**所見**:
- すべての副作用が `main` に集中しており、`internal/` 分割の出発点として `Render` 系の純粋関数化が必要。
- `dot` 必須チェックが冒頭にあるため、`erdm serve` での「dot 不在許容、SVG/PNG だけ 503」（要件 9.4）はサブコマンド分岐後に独立した起動シーケンスを設ける必要がある。

### 1.5 ビルド・配布

- `build.sh` の `go-bindata` 行は `templates_files.go` の `//go:embed` 化に伴い実質不要。**設計では `build.sh` を整理する**（`peg` 再生成 + `npm ci && npm run build` + `go build` の 3 段）。
- バイナリは `gox` で 4 ターゲット（linux/amd64, darwin/amd64, windows/amd64, windows/i386）。`embed.FS` でフロント `dist` を埋め込めば単一バイナリで配布可能。

## 2. 統合ポイント・既存パターン

| ポイント | 既存パターン | 新系統での扱い |
|---|---|---|
| 静的アセット同梱 | `templates_files.go` で `//go:embed templates/*.tmpl` | `internal/server` に `//go:embed all:frontend/dist` を追加し、同流儀を踏襲 |
| パーサ駆動 | PEG ジェネレータ（`pointlander/peg`）でステートフルレシーバ | `internal/parser` で同パターンを継続。レシーバは internal 専用化 |
| 出力テンプレート | `text/template` + `html/template` | `internal/dot` で `text/template` を継続。新規 `internal/elk` は `encoding/json` |
| プロセス内ロック | なし | `sync.Mutex` を `internal/server` に新設（要件 10.2） |
| 原子的置換 | なし | `os.CreateTemp` + `os.Rename`（要件 10.3） |
| Signal 処理 | なし | `signal.NotifyContext` + `http.Server.Shutdown`（要件 10.4） |

## 3. 外部技術調査

### 3.1 PEG ツールチェイン

- 既存 `erdm.peg.go` は `pointlander/peg` 系の自動生成物。`build.sh` には `peg erdm.peg` の呼び出しがある。
- **採択**: `pointlander/peg` を継続採用。CI ではバージョン固定するため `tools.go` 方式（`//go:build tools` ＋ `_ "github.com/pointlander/peg"`）または `Makefile` で `go install github.com/pointlander/peg@<vX.Y.Z>` を `setup` ステージに含める。
- **代替案不採用**: 別 PEG ジェネレータへの移行は `erdm.peg.go`（自動生成 66KB）の差し替えコストが大きく、本フィーチャースコープ外。

### 3.2 ELK / elkjs 入力スキーマ

- `elkjs` は `ElkNode` ツリーを入力とする。最小単位は `{ id, width, height, children?, edges?, layoutOptions?, properties? }`、エッジは `{ id, sources, targets }`。
- 階層化は `children` を使用する。primary group を表す親ノードを作り、子テーブルノードを `children` に格納する（要件 4.4）。
- secondary group は `properties.secondaryGroups: string[]` などのカスタム属性で表す（要件 4.5）。`elkjs` はカスタム属性を無視するため、レイアウト計算へ影響しない。
- ungrouped はルート直下 `children` に置く（要件 4.6）。

### 3.3 React Flow + elkjs リファレンスフロー

- React Flow 公式の examples に「ELK でレイアウトする」ガイドがある。`useNodesState`/`useEdgesState` + `elk.layout(graph)` の組合せで初期座標を計算し、`onNodeDragStop` で個別座標を更新する。
- 本機能では:
  1. 起動時に `/api/schema` + `/api/layout` を取得
  2. `layout.json` に座標がないノードのみ `elkjs` で初期配置を計算
  3. `onNodeDragStop` で位置変更を `PUT /api/layout`（debounce 推奨）

### 3.4 Vite + go:embed の DX

- 開発時は `frontend/` 内で `npm run dev`（Vite dev server）を別プロセスで起動し、Go バックエンドは `--frontend-proxy=http://localhost:5173` 等のフラグでリバースプロキシする戦略が一般的。**本フィーチャーではスコープを抑え、開発時もビルド版を `embed.FS` 経由で配信する**（DX より配布シンプルさを優先）。
- CI では `npm ci && npm run build` を `go build` の前段に必ず実行。`go generate` で連動させるかは設計判断とし、本設計では `Makefile`/`build.sh` の責務とする（`go generate` は冪等性が崩れやすいため不採用）。

### 3.5 `.erdm` シリアライザの正規化規則

要件 7.10（往復冪等性）を満たすため、シリアライザの規則を以下に固定する:

| 項目 | 規則 |
|---|---|
| 行頭テーブル宣言 | `<TitleReal>[/<Title>] [@groups[...]]` の順 |
| `@groups[...]` | 配列要素は引用符（`"..."`）で囲む。primary が先頭 |
| カラム宣言 | `<TitleReal>[/<Title>] [<Type>] ([NN] / [U] / [=default] / [-erd])* [<cardinality_left>--<cardinality_right> <relation_point>]` |
| カラム順 | 入力順を保持（パーサで配列化、シリアライズ時もそのまま出力） |
| インデックス | `index <name> (<col>, <col>) [unique]` |
| コメント行（`//`） | **保持しない**。要件 7.10 の対象は意味的同一性であり、コメントは設計でスコープ外と明記する |
| 空行 | テーブル間に 1 行入れる |

→ 設計時に `internal/serializer`（または `internal/parser` のサブパッケージ）で正規化規則を実装する。

### 3.6 `dot` 不在時のフォールバック（要件 9.4）

- `internal/server` の起動時に `exec.LookPath("dot")` を 1 度だけ実行し、結果（`hasDot bool`）をハンドラへ注入する。
- SVG/PNG エクスポートハンドラは `hasDot == false` のとき HTTP 503 + 説明 JSON を返す。
- 他のハンドラ（`/api/schema`, `/api/layout`, `/api/export/ddl`, SPA 配信）は dot 不在でも常に動作する。
- CLI 既定モード（`erdm [-output_dir DIR] schema.erdm`）は従来通り `dot` 必須を維持する。

### 3.7 既存 `html.tmpl` の扱い

- 旧 CLI（`erdm [-output_dir DIR] schema.erdm`）が生成する `<basename>.html` は PNG 埋込み HTML。
- 要件 9.1 で「従来同名出力」を保証する必要があるため、**`html.tmpl` は維持する**。Web UI（`erdm serve`）と並行存在させ、廃止しない。
- 設計上は `internal/html`（または `internal/dot` 内のサブモジュール）で旧 HTML 出力を担当する。

## 4. 要調査項目への結論

| 項目 | 結論 |
|---|---|
| PEG ツール固定 | `pointlander/peg` 継続。`tools.go` 方式でバージョン固定（CI でも `go install` 経由） |
| React Flow + elkjs | React Flow 公式の elkjs 連携パターンを採用（`onNodeDragStop` で `PUT /api/layout`） |
| frontend ビルド連動 | `Makefile` / `build.sh` で 3 段ビルド。`go generate` は不採用 |
| `.erdm` 正規化 | §3.5 の規則表に従う。コメント保持はスコープ外と明記 |
| `dot` 不在時のフォールバック | `exec.LookPath("dot")` を起動時に 1 度実行し、SVG/PNG ハンドラのみ 503 化 |
| 既存 `html.tmpl` の扱い | 並行維持（廃止しない）。Web UI と独立 |

## 4.5 設計レビューによる追加判断（2 回目以降の generate-design）

### 4.5.1 要件 7.7 の保存セマンティクス

- **判断**: `PUT /api/schema` は **受信テキストをバイト単位でそのまま** 元ファイルに保存する（再シリアライズしない）。
- **根拠**: 要件 7.7 の文言「受信した `.erdm` テキストを ... 元ファイルに上書き保存」、要件 7.6 の文言「SPA は内部スキーマモデルを `.erdm` テキストにシリアライズし `PUT /api/schema` に送信」。両者を整合させる唯一解はサーバ側で再シリアライズしない設計。
- **SPA 側責任**: `frontend/src/serializer/` が要件 7.6 のシリアライザの主体。研究 §3.5 の正規化規則に従う。
- **Go 側責任**: `internal/serializer` は要件 7.10（往復冪等性）の Go 単体テスト基盤、および将来の CLI 拡張への布石として維持。`PUT /api/schema` ハンドラからは呼び出さない。
- **整合性検証**: CI 上で「同一 `*Schema` JSON を入力したとき、Go の `internal/serializer.Serialize` と TS の `src/serializer.serialize` がバイト単位で一致する」クロスチェックテストを必須化する。

### 4.5.2 既存テンプレートの移送方針

- **判断**: リポジトリルート `templates/*.tmpl` を `internal/{dot,html,ddl}` 配下へ移送し、新フィールド名（`Schema/Table/Column` の新規プロパティ）で書き換える。アダプタ構造体は採用しない。
- **根拠**: モデル ↔ テンプレートの二重メンテナンスは保守負荷が高く、要件 3.2（`internal/model` の公開構造体定義）と要件 3.6（DOT 同等性）を両立する最も明快な方法はテンプレート側を新モデルに合わせて書き直すこと。
- **要件 3.6 の許容差分**: 要件 1.1〜1.7 由来の DOT 属性差分（`rankdir=LR`/`splines=ortho`/`nodesep=0.8`/`ranksep=1.2`/`concentrate=false` の追加）と親→子方向反転のみ。それ以外の差分は `internal/dot` のゴールデンファイルテストで検出する。
- **対応表**: design.md §テンプレートと新モデルのフィールド対応表 を一次資料とする。

### 4.5.3 `cmd` の `--format=elk` フラグ仕様

- **判断**: `runRender(args)` は `flag.NewFlagSet("render", ...)` で `-output_dir DIR` と `--format=dot|elk`（既定 `dot`）を解釈する。
- **`--format=dot`**: 旧 CLI 互換、`exec.LookPath("dot")` を必須チェック、5 種出力ファイルを生成。
- **`--format=elk`**: `dot` 必須チェックを行わない。`-output_dir` 指定時は `<output_dir>/<basename>.elk.json` に書き出し、未指定時は標準出力。
- **`runServe(args)`**: `--port=N`（既定 8080）、`--no-write`（既定 false）、`--listen=ADDR`（既定 `127.0.0.1`）。`exec.LookPath("dot")` の結果を `Server.Config.HasDot` として注入し、SVG/PNG ハンドラの 503 化に使用。

## 5. スコープ外（後続検討）

- `--focus <table> --depth N` 等のサブセット出力 CLI（`update.md` §6）
- Graphviz バックエンドの将来的な廃止判断
- 排他制御（複数ユーザー編集、Git 衝突回避）のサーバ間ロック
- Vite dev server とのリバースプロキシ統合 DX
