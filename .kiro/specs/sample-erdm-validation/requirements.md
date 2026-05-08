# requirements: sample-erdm-validation

## 導入

本feature `sample-erdm-validation` は、`erdm` CLI（render / serve 両モード）が `.erdm` DSL を入力として正しく動作することを、新規に作成するサンプル `.erdm` ファイルと検証スクリプトを用いてエンドツーエンドで確認するための機能である。サンプルファイルは DSL の主要構文（テーブル、PK、列型、`[NN]`/`[U]`/`[=default]`/`[-erd]`、関係カーディナリティ、`index ... unique`、`@groups[...]`、列コメント、タイトル）を網羅し、`erdm` の出力（DOT/PNG/HTML/PG SQL/SQLite SQL/ELK JSON）および `erdm serve` の HTTP API/SPA 配信が仕様どおりに振る舞うことを、人手検証と自動検証の両面で示すことを目的とする。

これにより、`erdm` の実装が「壊れていない」ことを継続的に確認できる成果物（サンプル、期待出力、検証スクリプト）を提供し、リグレッションを早期に発見できる土台を整える。

## 実装コンテキスト

- **既存実装の有無:** あり（部分的に既存）。
- **判定根拠:**
  - 既存 CLI 実装 `erdm.go`（`runRender` / `runServe`、要件 3.5/4.1/9.1/9.4/10.1/10.4 を実装済み）。
  - 既存パッケージ群 `internal/parser`, `internal/dot`, `internal/ddl`, `internal/html`, `internal/elk`, `internal/server`, `internal/layout`, `internal/serializer` が存在し、各出力レンダラを提供済み。
  - 既存サンプル `doc/sample/test.erdm`, `doc/sample/test_jp.erdm`, `doc/sample/test_no_logical_name.erdm`, `doc/sample/test_large_data_jp.erdm` が存在し、生成済み出力（`*.dot`/`*.png`/`*.html`/`*.pg.sql`/`*.sqlite3.sql`）も同梱されている。
  - 一方、本featureの主目的である「DSL 主要構文を体系的に網羅した検証用サンプル」と「エンドツーエンドで動作確認する検証スクリプト/期待出力」は未整備で、新規追加の対象となる。

## 要件

### 要件 1: 検証用サンプル `.erdm` ファイルの提供

**目的:** `erdm` 利用者および開発者として、DSL 主要構文を網羅した検証用 `.erdm` ファイルを `doc/sample/` 配下から取得し、`erdm` の挙動確認とリグレッション検出を実行したい。

#### 受け入れ条件

1.1 サンプルファイルを配置する場合、`erdm` リポジトリは `doc/sample/validation_basic.erdm` を提供しなければならない。

1.2 サンプルファイルを配置する場合、`erdm` リポジトリは `doc/sample/validation_full.erdm` を提供しなければならない。

1.3 `validation_basic.erdm` を読み込んだとき、`internal/parser.Parse` はエラーを返してはならない。

1.4 `validation_full.erdm` を読み込んだとき、`internal/parser.Parse` はエラーを返してはならない。

1.5 `validation_full.erdm` の内容として、`erdm` リポジトリは `# Title:` 宣言を1行目に含めなければならない。

1.6 `validation_full.erdm` の内容として、`erdm` リポジトリは `+name` 形式の主キー列を少なくとも1つ含むテーブルを最低3個含めなければならない。

1.7 `validation_full.erdm` の内容として、`erdm` リポジトリは `[NN]`, `[U]`, `[=default]`, `[-erd]` の各列属性を最低1回ずつ使用しなければならない。

1.8 `validation_full.erdm` の内容として、`erdm` リポジトリは `0..*--1`, `1--0..1`, `1--1` のいずれか3種以上のカーディナリティ表記を最低1回ずつ含めなければならない。

1.9 `validation_full.erdm` の内容として、`erdm` リポジトリは `index <名前> (col1, col2)` 形式の複合インデックス宣言を最低1個含めなければならない。

1.10 `validation_full.erdm` の内容として、`erdm` リポジトリは `index <名前> (...) unique` 形式のユニーク・インデックス宣言を最低1個含めなければならない。

1.11 `validation_full.erdm` の内容として、`erdm` リポジトリは `@groups["X", "Y"]` 形式の複数グループ宣言を最低1個含めなければならない。

1.12 `validation_full.erdm` の内容として、`erdm` リポジトリは列単位コメント（`# ...` を列定義の直後に記述）を最低1個含めなければならない。

1.13 `validation_full.erdm` の内容として、`erdm` リポジトリは論理名なしの列（`name [type]` 形式）を最低1個含めなければならない。

### 要件 2: render モード（DOT 形式）の動作検証

**目的:** `erdm` 利用者として、サンプル `.erdm` を `--format=dot`（既定）で render 実行したときに、5種類（DOT/PNG/HTML/PG SQL/SQLite SQL）の出力が `-output_dir` 配下に生成されることを保証したい。

#### 受け入れ条件

2.1 `dot` コマンドが PATH 上に存在する状態で `erdm -output_dir <DIR> doc/sample/validation_full.erdm` が実行されたとき、`erdm` CLI は終了コード 0 を返さなければならない。

2.2 上記 2.1 の実行が成功したとき、`erdm` CLI は `<DIR>/validation_full.dot` を生成しなければならない。

2.3 上記 2.1 の実行が成功したとき、`erdm` CLI は `<DIR>/validation_full.png` を生成しなければならない。

2.4 上記 2.1 の実行が成功したとき、`erdm` CLI は `<DIR>/validation_full.html` を生成しなければならない。

2.5 上記 2.1 の実行が成功したとき、`erdm` CLI は `<DIR>/validation_full.pg.sql` を生成しなければならない。

2.6 上記 2.1 の実行が成功したとき、`erdm` CLI は `<DIR>/validation_full.sqlite3.sql` を生成しなければならない。

2.7 生成された `<DIR>/validation_full.dot` を `dot -Tpng -o /dev/null` で再描画したとき、`dot` コマンドは終了コード 0 を返さなければならない。

2.8 生成された `<DIR>/validation_full.pg.sql` を PostgreSQL 互換パーサ（`pgx` / `psql --set ON_ERROR_STOP=on -f` 等）に投入したとき、構文エラーは0件でなければならない。

2.9 生成された `<DIR>/validation_full.sqlite3.sql` を `sqlite3 :memory:` に投入したとき、SQLite は終了コード 0 を返さなければならない。

2.10 `dot` コマンドが PATH 上に存在しない状態で `erdm -output_dir <DIR> doc/sample/validation_full.erdm` が実行された場合、`erdm` CLI は標準エラーへ `dot command not found in PATH` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

### 要件 3: render モード（ELK 形式）の動作検証

**目的:** `erdm` 利用者として、`--format=elk` でレイアウト用 ELK JSON を取得でき、`-output_dir` 指定の有無で出力先（標準出力/ファイル）が切り替わることを保証したい。

#### 受け入れ条件

3.1 `erdm --format=elk doc/sample/validation_full.erdm` が `-output_dir` を指定せずに実行されたとき、`erdm` CLI は標準出力へ ELK JSON を書き出し、終了コード 0 を返さなければならない。

3.2 上記 3.1 の標準出力を `encoding/json` でデコードしたとき、JSON パースエラーは発生してはならない。

3.3 `erdm --format=elk -output_dir <DIR> doc/sample/validation_full.erdm` が実行されたとき、`erdm` CLI は `<DIR>/validation_full.elk.json` を生成し、終了コード 0 を返さなければならない。

3.4 上記 3.3 が実行されたとき、`erdm` CLI は標準出力へ ELK JSON を書き出してはならない。

3.5 `--format=elk` が指定されたとき、`erdm` CLI は `dot` コマンドの存在検査を行ってはならない（`dot` 不在環境でも終了コード 0 を返さなければならない）。

3.6 不正な `.erdm` を入力として `erdm --format=elk <path>` が実行された場合、`erdm` CLI は標準エラーへ `parse <path>:` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

### 要件 4: serve モード（HTTP API/SPA）の動作検証

**目的:** `erdm` 利用者として、`erdm serve` でサンプルファイルをロードし、ブラウザおよび HTTP クライアントから API/SPA に正しくアクセスできることを保証したい。

#### 受け入れ条件

4.1 `erdm serve --port=<P> --listen=127.0.0.1 doc/sample/validation_full.erdm` が起動されたとき、`erdm` CLI は `127.0.0.1:<P>` で TCP リッスンを開始しなければならない。

4.2 上記 4.1 の状態で `GET /` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 と `Content-Type: text/html` のレスポンスを返さなければならない。

4.3 上記 4.1 の状態で `GET /api/schema` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 と `application/json` を返し、レスポンス本文（JSON）に `validation_full.erdm` 由来のテーブル名（`users` 等）を JSON 文字列値として含めなければならない。

4.4 上記 4.1 の状態で `GET /api/layout` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 と `application/json` を返さなければならない。

4.5 上記 4.1 の状態で `GET /api/export/ddl?dialect=pg` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 を返し、レスポンス本文に `CREATE TABLE` を含めなければならない。

4.6 上記 4.1 の状態で `GET /api/export/ddl?dialect=sqlite3` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 を返し、レスポンス本文に `CREATE TABLE` を含めなければならない。

4.7 `dot` コマンドが PATH 上に存在する状態で `GET /api/export/svg` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 と `Content-Type: image/svg+xml` を返さなければならない。

4.8 `dot` コマンドが PATH 上に存在する状態で `GET /api/export/png` が要求されたとき、`erdm` HTTP サーバはステータスコード 200 と `Content-Type: image/png` を返さなければならない。

4.9 `dot` コマンドが PATH 上に存在しない状態で `GET /api/export/svg` が要求された場合、`erdm` HTTP サーバはステータスコード 503 を返さなければならない。

4.10 `--no-write` フラグ付きで起動された状態で `PUT /api/schema` が要求された場合、`erdm` HTTP サーバはステータスコード 403 を返さなければならない。

4.11 `--no-write` フラグ付きで起動された状態で `PUT /api/layout` が要求された場合、`erdm` HTTP サーバはステータスコード 403 を返さなければならない。

4.12 `--no-write` フラグなしで起動された状態で正当な DSL 本文を `PUT /api/schema` で受信したとき、`erdm` HTTP サーバはステータスコード 200 を返し、入力ファイル `validation_full.erdm` の内容を更新しなければならない。

4.13 `--no-write` フラグなしで起動された状態で構文不正な DSL 本文を `PUT /api/schema` で受信した場合、`erdm` HTTP サーバはステータスコード 400 を返し、入力ファイル `validation_full.erdm` の内容を変更してはならない。

### 要件 5: 異常系入力の取り扱い

**目的:** `erdm` 利用者として、入力ファイルが不在/ディレクトリ/構文エラーの場合に、原因を特定できる明確なエラーメッセージと非ゼロ終了コードで失敗することを保証したい。

#### 受け入れ条件

5.1 入力ファイルが存在しない状態で `erdm <存在しないパス>` が実行された場合、`erdm` CLI は標準エラーへ `input file:` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

5.2 入力ファイルとしてディレクトリパスが渡された場合、`erdm` CLI は標準エラーへ `is a directory` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

5.3 構文不正な `.erdm`（例: 列型 `[]` 欠落）を入力として `erdm` が実行された場合、`erdm` CLI は標準エラーへ `parse` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

5.4 `erdm --format=<未知の値> doc/sample/validation_full.erdm` が実行された場合、`erdm` CLI は標準エラーへ `unknown format:` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

5.5 引数なしで `erdm` が実行された場合、`erdm` CLI は標準エラーへ `Usage: erdm` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

5.6 引数なしで `erdm serve` が実行された場合、`erdm` CLI は標準エラーへ `Usage: erdm serve` を含む文字列を出力し、非ゼロ終了コードを返さなければならない。

### 要件 6: 検証スクリプトとリグレッション検出

**目的:** `erdm` 開発者として、サンプルに対する一連の検証手順をワンコマンドで再実行し、CI を含む環境で動作確認できる検証スクリプトを保有したい。

#### 受け入れ条件

6.1 検証スクリプトを提供する場合、`erdm` リポジトリは `scripts/validate_sample.sh` を提供しなければならない。

6.2 `scripts/validate_sample.sh` が `dot` コマンド存在環境で実行されたとき、`scripts/validate_sample.sh` は終了コード 0 を返さなければならない。

6.3 `scripts/validate_sample.sh` が実行されたとき、`scripts/validate_sample.sh` は要件 2.1〜2.9 を順次検証しなければならない。

6.4 `scripts/validate_sample.sh` が実行されたとき、`scripts/validate_sample.sh` は要件 3.1〜3.5 を順次検証しなければならない。

6.5 `scripts/validate_sample.sh` が実行されたとき、`scripts/validate_sample.sh` は `erdm serve` を一時ポートで起動し、要件 4.2〜4.6 および 4.10〜4.11 を検証してから停止しなければならない。

6.6 `scripts/validate_sample.sh` が検証中に1件でも失敗した場合、`scripts/validate_sample.sh` は失敗内容を標準エラーに出力し、非ゼロ終了コードを返さなければならない。

6.7 `scripts/validate_sample.sh` は常に作業用一時ディレクトリを作成・利用し、終了時にクリーンアップしなければならない。

6.8 `dot` コマンドが PATH 上に存在しない場合、`scripts/validate_sample.sh` は要件 2.x のうち `dot` 必須項目をスキップ扱いとし、要件 3.x（ELK）と要件 5.x（異常系）のみを検証して終了コード 0 を返さなければならない。

### 要件 7: 既存サンプルとの互換性維持

**目的:** `erdm` 利用者として、本feature導入後も既存サンプル（`doc/sample/test.erdm` 等）の挙動が変更されないことを保証したい。

#### 受け入れ条件

7.1 本feature実装は常に `doc/sample/test.erdm`, `doc/sample/test_jp.erdm`, `doc/sample/test_no_logical_name.erdm`, `doc/sample/test_large_data_jp.erdm` のファイル内容を変更してはならない。

7.2 本feature実装は常に既存パッケージ `internal/parser`, `internal/dot`, `internal/ddl`, `internal/html`, `internal/elk`, `internal/server`, `internal/layout`, `internal/serializer` の公開 API シグネチャを変更してはならない。

7.3 本feature実装後に `make test` が実行されたとき、Go テストは終了コード 0 を返さなければならない。

## 要件間の依存関係

- 要件 2/3/4 は要件 1（サンプルファイル）に依存する。要件 1 のサンプルが存在しない場合、要件 2/3/4 は検証不能となる。
- 要件 6（検証スクリプト）は要件 1〜5 を実行可能な前提条件に依存する。
- 要件 7（互換性維持）は要件 1〜6 と並行する制約条件であり、いずれかの要件実装が要件 7 に違反する場合、当該要件実装は採用してはならない。
