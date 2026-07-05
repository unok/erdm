# 要件定義: ELK + Web UI 移行 (elk-webui-migration)

## 導入

`erdm` は `.erdm` DSL から ERD を生成する CLI ツールであり、現在は Graphviz/DOT を用いて PNG/HTML/SQL を出力している。本フィーチャーでは、`update.md` で合意された方針に従い、ERD 可読性向上を目的として「DOT 出力品質の改善」「DSL の `@groups[...]` 拡張」「ELK + React Flow ベースの Web UI への新デフォルト移行」を段階的に実装する。最終形では `erdm serve` サブコマンドにより、ブラウザ上で ERD の閲覧・手動レイアウト調整・`.erdm` 編集・エクスポートが行える単一バイナリ配布のローカルアプリケーションとなる。

レガシーの DOT バックエンドは互換のため当面並行維持し、新系統 (ELK / Web UI) を新デフォルトとして導入する。

## 実装コンテキスト

- **既存実装**: あり（部分的）。
- **判定根拠**:
  - `erdm.go` にメインロジック（パース実行、DOT/HTML/PG/SQLite テンプレート出力）が単一パッケージで集約されている。
  - `erdm.peg` / `erdm.peg.go` が現行 PEG パーサ（`@groups[...]` 構文未対応）を提供している。
  - `templates/` 配下に DOT・HTML・DDL の `text/template` / `html/template` テンプレートが存在する（`rankdir`・`splines`・cluster 未指定）。
  - `internal/`, `frontend/`, `erdm serve` サブコマンド、レイアウト JSON 保存機能はいずれも未実装。

## 要件

### 1. DOT 出力の可読性改善 (Phase 1)

**目的:** ERD 利用者として、既存の DOT バックエンドのまま図の可読性が向上した出力を得たい。レイアウト方向・直交ルーティング・親子方向の正規化により、図中の交差や曲線を減らしたい。

#### 受け入れ条件

1.1 DOT 出力グラフが生成されるとき、`erdm` は DOT 属性 `rankdir` を `LR` に設定して出力しなければならない。

1.2 DOT 出力グラフが生成されるとき、`erdm` は DOT 属性 `splines` を `ortho` に設定して出力しなければならない。

1.3 DOT 出力グラフが生成されるとき、`erdm` は DOT 属性 `nodesep` を `0.8` に設定して出力しなければならない。

1.4 DOT 出力グラフが生成されるとき、`erdm` は DOT 属性 `ranksep` を `1.2` に設定して出力しなければならない。

1.5 DOT 出力グラフが生成されるとき、`erdm` は DOT 属性 `concentrate` を `false` に設定して出力しなければならない。

1.6 `.erdm` のカラムが他テーブルへの FK を持つとき、`erdm` は DOT のエッジを「参照される側 (親テーブル) → 参照する側 (子テーブル)」の方向で出力しなければならない。

1.7 同一の親子関係を表す FK が複数カラムに存在する場合、`erdm` は各 FK エッジを独立した `親 -> 子` のエッジとして DOT に出力しなければならない（重複統合は行わない）。

1.8 DOT 出力に `WithoutErd` 指定 (`-erd`) のカラムが含まれる場合、`erdm` は当該カラムから派生するエッジを DOT 出力に含めてはならない。

1.9 既存テストスナップショットが存在する場合、`erdm` のテストスイートは新しい既定値（`rankdir=LR`/`splines=ortho`/親→子方向）で再生成されたスナップショットと一致しなければならない。

### 2. DSL 拡張: `@groups[...]` 構文 (Phase 2)

**目的:** ERD 設計者として、テーブルに複数の意味的グループを宣言し、図上の cluster やフィルタに利用したい。

#### 受け入れ条件

2.1 `.erdm` のテーブル宣言行に `@groups["A", "B"]` の形式が記述されたとき、`erdm` のパーサは当該テーブルに `["A", "B"]` の順序付き group 配列を関連付けなければならない。

2.2 `.erdm` のテーブル宣言行に `@groups[...]` が **記述されていない** とき、`erdm` のパーサは当該テーブルを「ungrouped」として扱わなければならない。

2.3 `@groups[...]` の配列要素が 1 つの場合、`erdm` のパーサは当該要素を primary group としてのみ保持しなければならない。

2.4 `@groups[...]` の配列要素が 2 つ以上の場合、`erdm` のパーサは配列の **先頭要素を primary group**、それ以降を secondary group として保持しなければならない。

2.5 `@groups[...]` の角括弧内が空 (`@groups[]`) の場合、`erdm` のパーサは構文エラーとして検知し、エラー位置を伴うメッセージで停止しなければならない。

2.6 `@groups["X"]` の引用符が閉じていない、またはカンマ区切りが不正な場合、`erdm` のパーサは構文エラーとして検知し、エラー位置を伴うメッセージで停止しなければならない。

2.7 group 名の文字列リテラルが `.erdm` 内で初出のとき、`erdm` は登場順を保持して group 一覧を構築しなければならない。

2.8 トップレベル `group { ... }` ブロックが `.erdm` 内に記述された場合、`erdm` のパーサは構文エラーとして検知しなければならない（本フィーチャーでは `@groups[...]` のみサポート）。

2.9 同一テーブル宣言中に `@groups[...]` が 2 回以上記述された場合、`erdm` のパーサは構文エラーとして検知しなければならない。

2.10 `@groups[...]` の指定されたテーブルを DOT 出力するとき、`erdm` は primary group ごとに `subgraph cluster_<group名>` を生成し、当該テーブルノードを cluster 内に配置しなければならない。

2.11 `@groups[...]` の secondary group は、DOT 出力には現れてはならない（cluster の二重所属を避けるため）。

2.12 `ungrouped` テーブルが存在する場合、`erdm` の DOT 出力は当該テーブルをいずれの cluster にも所属させてはならない。

### 3. 内部モデルとパッケージ分割 (Phase 3)

**目的:** 開発者として、現状 `erdm.go` 単一ファイルに集約された処理を責務単位のパッケージに切り出し、テスト容易性と将来拡張性を確保したい。

#### 受け入れ条件

3.1 リファクタリングが完了したとき、`erdm` リポジトリには以下のパッケージが存在しなければならない: `internal/model`, `internal/parser`, `internal/dot`, `internal/elk`, `internal/layout`, `internal/server`。

3.2 `internal/model` パッケージは、テーブル・カラム・FK・group・index を表現する Go 構造体を公開しなければならない。

3.3 `internal/parser` パッケージは、`erdm.peg.go` をラップし `[]byte` 入力から `internal/model` 構造体を返す関数を公開しなければならない。

3.4 `internal/dot` パッケージは、`internal/model` の値を入力として DOT テキストを生成する関数を公開しなければならない。

3.5 リファクタリング後、`erdm` CLI を従来同様 `erdm [-output_dir DIR] schema.erdm` の引数で起動したとき、`erdm` は従来と等価な `*.dot` / `*.png` / `*.html` / `*.pg.sql` / `*.sqlite3.sql` ファイルを `output_dir` に生成しなければならない（後方互換）。

3.6 リファクタリング前後で同一の `.erdm` 入力を処理した場合、`erdm` の DOT 出力は要件 1 で改善された属性差分を除き、テーブル・FK・index 構造において意味的に同一でなければならない。

### 4. ELK JSON 出力 (Phase 4)

**目的:** Web UI 開発者および CLI デバッグ利用者として、内部モデルから ELK レイアウトエンジン用 JSON を取得し、フロントエンドや ELK CLI に渡したい。

#### 受け入れ条件

4.1 `erdm --format=elk schema.erdm` が実行されたとき、`erdm` は標準出力 (もしくは `-output_dir` 配下の `.elk.json`) に ELK 互換の JSON を出力しなければならない。

4.2 ELK JSON が生成されるとき、`erdm` は各テーブルを `id` / `width` / `height` を持つ ELK ノードとして出力しなければならない。

4.3 ELK JSON が生成されるとき、`erdm` は各 FK を `id` / `sources` / `targets` を持つ ELK エッジとして親→子方向で出力しなければならない。

4.4 `.erdm` 中のテーブルに primary group が指定されている場合、`erdm` は ELK JSON において当該 primary group を `children` を持つ親ノード（groupNode）として表現し、対応するテーブルノードを当該親ノードの `children` 配列に格納しなければならない。

4.5 ELK JSON が生成されるとき、`erdm` は secondary group を当該テーブルノードのカスタム属性（例: `properties.secondaryGroups`）として配列で保持しなければならない。

4.6 `ungrouped` テーブルが存在する場合、`erdm` は ELK JSON のルート要素直下の `children` 配列に当該ノードを配置しなければならない。

4.7 ELK JSON のスキーマは `elkjs` の標準入力フォーマットを満たし、`elkjs` でレイアウト計算を実行した際にエラーを発生させてはならない。

### 5. `erdm serve` サブコマンド (Phase 5)

**目的:** エンドユーザーとして、`erdm serve schema.erdm` を実行するだけでブラウザ上に ERD を表示し、閲覧・ズーム・パンを行いたい。

#### 受け入れ条件

5.1 `erdm serve <schema.erdm>` が実行されたとき、`erdm` は HTTP サーバを起動し、既定ポート `8080` で `0.0.0.0` または `127.0.0.1` をリッスンしなければならない。

5.2 `erdm serve <schema.erdm> --port=<N>` が実行されたとき、`erdm` は HTTP サーバを指定ポート `<N>` でリッスンしなければならない。

5.3 `erdm serve` 起動後、HTTP サーバが `GET /` を受信したとき、`erdm` は `embed.FS` に同梱された SPA の `index.html` を返さなければならない。

5.4 `erdm serve` 起動後、HTTP サーバが `GET /api/schema` を受信したとき、`erdm` は対象 `.erdm` を再パースして得たスキーマを JSON で返さなければならない。

5.5 `erdm serve` 起動後、HTTP サーバが `GET /api/layout` を受信したとき、`erdm` は隣接する `<schema>.erdm.layout.json` を読み取り、存在すれば内容を JSON で返し、存在しなければ HTTP 200 と空オブジェクト `{}` を返さなければならない。

5.6 `erdm serve` 起動後、HTTP サーバが `GET /api/export/ddl` を受信したとき、`erdm` は PG または SQLite3 の DDL（クエリパラメータ `dialect=pg|sqlite3`、既定 `pg`）を `text/plain` で返さなければならない。

5.7 `erdm serve` 起動後、HTTP サーバが `GET /api/export/svg` を受信したとき、`erdm` は現スキーマから生成した SVG を `image/svg+xml` で返さなければならない。

5.8 `erdm serve` 起動後、HTTP サーバが `GET /api/export/png` を受信したとき、`erdm` は現スキーマから生成した PNG を `image/png` で返さなければならない。

5.9 `erdm serve` 実行中に対象 `.erdm` ファイルが他プロセスから削除された状態で `GET /api/schema` が受信された場合、`erdm` は HTTP 500 とエラーメッセージ JSON を返さなければならない。

5.10 SPA は React + React Flow + elkjs を用いて実装され、初期ロード時に `/api/schema` および `/api/layout` を取得し、ELK で初期レイアウトを計算したうえでテーブルノードと FK エッジを描画しなければならない。

5.11 SPA はマウスホイール・トラックパッドのピンチ操作によりズーム可能でなければならず、ドラッグ操作によりキャンバスをパン可能でなければならない。

5.12 フロントエンドのバンドル成果物は Go の `embed.FS` で同梱され、`erdm` バイナリ単体で配布・起動可能でなければならない。

### 6. レイアウト座標の保存と復元 (Phase 6)

**目的:** ERD 編集者として、Web UI 上で手動調整したテーブル配置を保存し、次回起動時に再現したい。

#### 受け入れ条件

6.1 SPA がテーブルノードのドラッグ完了イベントを受信したとき、SPA は更新後の座標一覧を `PUT /api/layout` に JSON ボディで送信しなければならない。

6.2 `erdm serve` 起動後、HTTP サーバが `PUT /api/layout` を受信し `--no-write` フラグが指定されていないとき、`erdm` は受信した JSON を `<schema>.erdm.layout.json` に書き込み、HTTP 200 を返さなければならない。

6.3 `erdm serve --no-write` モード時に `PUT /api/layout` を受信した場合、`erdm` は HTTP 403 を返さなければならない。

6.4 SPA の起動時、SPA は `GET /api/layout` で取得した座標が当該テーブルに対して存在する場合、ELK の自動配置結果より優先して当該座標でノードを描画しなければならない。

6.5 `<schema>.erdm.layout.json` に座標が **存在しないテーブル** が `.erdm` に新規追加された場合、SPA は当該テーブルを ELK の自動配置結果に従ってフォールバック配置しなければならない。

6.6 `<schema>.erdm.layout.json` が破損した JSON を含む場合、`erdm` は HTTP 500 とエラーメッセージを `GET /api/layout` のレスポンスとして返さなければならず、サーバプロセスを停止してはならない。

### 7. `.erdm` 編集機能 (Phase 7)

**目的:** ERD 設計者として、Web UI からテーブル・カラム・FK・`@groups` を追加・編集・削除し、`.erdm` テキストとして保存したい。

#### 受け入れ条件

7.1 SPA はテーブル追加・編集・削除のフォーム UI を提供しなければならない。

7.2 SPA はカラム追加・編集・削除のフォーム UI を提供しなければならず、PK/NN/UNIQUE/FK/Default/コメント/`-erd` フラグの編集が可能でなければならない。

7.3 SPA は FK の追加・編集・削除を、参照先テーブル選択と cardinality 指定により可能としなければならない。

7.4 SPA は `@groups[...]` の編集（primary/secondary の入れ替え・追加・削除）を可能としなければならない。

7.5 SPA は編集中の状態をブラウザの `localStorage` に下書きとして自動保存しなければならず、ページリロード後も下書きを復元できなければならない。

7.6 ユーザーが SPA の「保存」アクションを実行したとき、SPA は内部スキーマモデルを `.erdm` テキストにシリアライズし `PUT /api/schema` に送信しなければならない。

7.7 `erdm serve` 起動後、HTTP サーバが `PUT /api/schema` を受信し `--no-write` フラグが指定されていないとき、`erdm` は受信した `.erdm` テキストを起動時に指定された元ファイルに上書き保存し、HTTP 200 を返さなければならない。

7.8 `erdm serve --no-write` モード時に `PUT /api/schema` を受信した場合、`erdm` は HTTP 403 を返さなければならない。

7.9 `PUT /api/schema` で受信した `.erdm` テキストがパースエラーとなる場合、`erdm` は元ファイルを書き換えてはならず、HTTP 400 とエラー位置情報を含む JSON を返さなければならない。

7.10 `.erdm` テキストへのシリアライズ結果は、再パース後にシリアライズ前と意味的に同一の内部モデルを生成しなければならない（往復変換の冪等性）。

### 8. エクスポート UI とドキュメント整備 (Phase 8)

**目的:** ERD 利用者として、Web UI から DDL / SVG / PNG をワンクリックでダウンロードし、現行ドキュメントから新フローを学習したい。

#### 受け入れ条件

8.1 SPA はメニューもしくはボタンから DDL ダウンロードを可能としなければならず、ダイアレクト選択（PostgreSQL / SQLite3）を提供しなければならない。

8.2 ユーザーが SPA の DDL ダウンロードボタンを押下したとき、SPA は `GET /api/export/ddl?dialect=...` の応答をファイルとしてブラウザにダウンロードさせなければならない。

8.3 ユーザーが SPA の SVG ダウンロードボタンを押下したとき、SPA は `GET /api/export/svg` の応答を `<basename>.svg` としてダウンロードさせなければならない。

8.4 ユーザーが SPA の PNG ダウンロードボタンを押下したとき、SPA は `GET /api/export/png` の応答を `<basename>.png` としてダウンロードさせなければならない。

8.5 リポジトリの `README.md` には、`erdm serve` を新デフォルトフローとする手順、Graphviz バックエンドの位置付け（互換のため並行維持）、`@groups[...]` 構文例が追記されていなければならない。

8.6 リポジトリのドキュメントは、Web UI 編集と Git 管理の運用注意点（`.erdm` と `*.erdm.layout.json` を共にコミット対象とする旨）を明記しなければならない。

### 9. 互換性・配布・運用 (横断要件)

**目的:** 既存ユーザーとして、本改修後も従来の CLI 利用方法・出力ファイルが破壊されないことを保証されたい。

#### 受け入れ条件

9.1 `erdm [-output_dir DIR] schema.erdm` 形式の従来 CLI が実行されたとき、`erdm` は要件 1 の DOT 改善を反映したうえで従来と同名の出力ファイル群（`.dot`/`.png`/`.html`/`.pg.sql`/`.sqlite3.sql`）を生成しなければならない。

9.2 既存 `.erdm` ファイルに `@groups[...]` が含まれていない場合、`erdm` のパーサは当該ファイルを従来通りパース成功させなければならない（後方互換）。

9.3 `erdm serve` のバイナリ配布は単一の Go 実行ファイルでなければならず、フロントエンド資産は `embed.FS` を通じて埋め込まれていなければならない。

9.4 `erdm serve` 起動時に Graphviz の `dot` コマンドが PATH 上に存在しない場合、`erdm` は SVG/PNG エクスポート API のみ HTTP 503 を返さなければならず、その他 API およびフロント表示は引き続き動作しなければならない。

9.5 `erdm` のテストスイートは、`internal/parser` パッケージで `@groups[...]` を含むケースとそれを含まないケースの双方が成功する単体テストを備えていなければならない。

9.6 `erdm` のテストスイートは、`internal/dot` パッケージで「primary group ありテーブル」「secondary group のみのテーブル」「ungrouped テーブル」の DOT 出力スナップショットを備えていなければならない。

### 10. エラー処理と運用安全性 (横断要件)

**目的:** 運用者として、想定外のファイル状態・破壊的編集から保護されたい。

#### 受け入れ条件

10.1 `erdm serve` 起動時に対象 `.erdm` ファイルが存在しない、もしくは読み取り権限がない場合、`erdm` は標準エラーにエラーメッセージを出力して終了コード非ゼロで終了しなければならない。

10.2 `PUT /api/schema` および `PUT /api/layout` の同時並行リクエストが受信された場合、`erdm` はファイル書き込みを直列化（プロセス内ロック）しなければならず、ファイル破損を発生させてはならない。

10.3 `PUT /api/schema` 実行時にディスク書き込みエラーが発生した場合、`erdm` は元ファイルを破壊してはならず（一時ファイル経由で原子的に置換）、HTTP 500 を返さなければならない。

10.4 `erdm serve` の HTTP サーバは、SIGINT または SIGTERM を受信した場合、進行中のリクエスト完了を待ってからプロセスを終了しなければならない（graceful shutdown）。

## 要件間の依存関係

| 要件 | 依存先 |
| --- | --- |
| 2 (DSL 拡張) | 1 (DOT 改善は前提ではないが、2.10–2.12 の DOT 出力検証は 1 と整合する必要あり) |
| 3 (パッケージ分割) | 1, 2 (改善・拡張済みロジックを切り出すため) |
| 4 (ELK 出力) | 3 (`internal/model` への依存) |
| 5 (`erdm serve`) | 3, 4 (HTTP ハンドラが内部モデル・ELK 変換器を利用) |
| 6 (レイアウト保存) | 5 (HTTP API 基盤) |
| 7 (`.erdm` 編集) | 3, 5 (内部モデル ↔ `.erdm` 双方向変換および API) |
| 8 (エクスポート UI/Doc) | 5, 6, 7 (UI 完成の総仕上げ) |
| 9 (互換性) | 1–8 すべての変更後に破綻していないことを保証 |
| 10 (エラー処理) | 5, 6, 7 (HTTP API 関連の安全性) |
