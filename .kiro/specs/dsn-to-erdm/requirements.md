# DSN 指定による erdm ファイル生成機能 要件定義

## 導入

PostgreSQL / SQLite / MySQL の稼働中データベースへ DSN（Data Source Name）で接続し、内部スキーマ情報（テーブル・カラム・主キー・外部キー・インデックス・ユニーク制約・コメント等）を取得して、既存パーサが受理する `.erdm` 形式テキストへ変換出力する CLI 機能を新設する。これにより、既存の DB 設計資産を素早く ER 図化（render モードで `.dot/.png/.html` 等を生成）できるようにし、後段の `serve` モードでの編集ループにも接続できる。

論理名（`/論理名` 表記）は、DB から取得できるテーブル/カラムコメントを基本値として採用し、コメント未設定の場合は物理名をそのまま論理名としても出力することで、生成直後の `.erdm` でも図中の論理名表示が空にならないことを保証する。

## 実装コンテキスト

- **既存実装の有無:** なし
- **判定根拠:**
  - `erdm.go` の CLI は `render` / `serve` の 2 系統のみで、DSN 由来の入力経路を持たない（`erdm.go:42-58`）。
  - `internal/` 配下に `parser` / `serializer` / `model` / `ddl` / `dot` / `elk` / `html` / `server` / `layout` パッケージは存在するが、データベースへ接続して内部スキーマを取得するパッケージ（例: `internal/introspect` 等）は存在しない（`internal/` ディレクトリ確認）。
  - `go.mod` には DB ドライバ（`pgx` / `mysql` / `sqlite` 系）の依存が含まれていない（`go.mod:1-11`）。

## 要件

### 1. CLI サブコマンドとしての提供

**目的:** ERDM 利用者として、既存の `render` / `serve` を壊さずに DSN から `.erdm` を生成する経路を追加し、同じバイナリで完結したい。

#### 受け入れ条件

1.1 `erdm` バイナリの第 1 引数に `import` が指定されたとき、`erdm` CLI は import モードへ分岐し、render / serve のフラグ解析をスキップしなければならない。

1.2 `import` サブコマンドが `--dsn` フラグと `--driver` フラグを受け取った場合、`erdm` CLI は両者の値を取得し以降の処理に渡さなければならない。

1.3 `import` サブコマンドが `--out` フラグで出力ファイルパスを受け取った場合、`erdm` CLI は生成した `.erdm` テキストを当該パスへ書き出さなければならない。

1.4 `import` サブコマンドの `--out` フラグが省略された場合、`erdm` CLI は生成した `.erdm` テキストを標準出力へ書き出さなければならない。

1.5 `import` サブコマンドの `--driver` フラグが省略された場合、`erdm` CLI は `--dsn` の値からドライバ種別を推定（`postgres://` / `postgresql://` → postgres、`mysql://` → mysql、`file:` または拡張子 `.db` / `.sqlite` / `.sqlite3` → sqlite）しなければならない。

1.6 `import` サブコマンドの `--dsn` フラグが空文字列または未指定の場合、`erdm` CLI は終了コード非ゼロで `usage: erdm import --driver=postgres|mysql|sqlite --dsn=<DSN> [--out=PATH]` を標準エラーへ出力しなければならない。

1.7 既存の `render` / `serve` サブコマンドの動作は、本要件の追加によって変更されてはならない。

### 2. ドライバ別 DSN の受理

**目的:** 利用者として、各 DBMS で一般的に流通している DSN 表記をそのまま渡したい。

#### 受け入れ条件

2.1 `--driver=postgres` のとき、`erdm import` は `postgres://user:pass@host:port/dbname?sslmode=...` 形式の URL DSN を受理しなければならない。

2.2 `--driver=mysql` のとき、`erdm import` は Go の `database/sql` 標準で広く使われる `user:pass@tcp(host:port)/dbname?param=value` 形式の DSN を受理しなければならない。

2.3 `--driver=sqlite` のとき、`erdm import` は SQLite ファイルパス（例: `/tmp/foo.db`）または `file:` スキーム形式の DSN を受理しなければならない。

2.4 サポート外の `--driver` 値が指定された場合、`erdm import` は終了コード非ゼロで「unsupported driver: <値>」を標準エラーへ出力し、DB 接続を試みてはならない。

2.5 受理した DSN で DB 接続に失敗した場合、`erdm import` はエラーメッセージから DSN 内のパスワード文字列をマスク（`***` へ置換）した上で、標準エラーへ出力しなければならない。

### 3. テーブル一覧の取得

**目的:** 利用者として、対象 DB に存在するユーザー定義テーブルすべてを `.erdm` の Table として出力したい。

#### 受け入れ条件

3.1 `erdm import` は接続先 DB から、システムスキーマ（PostgreSQL の `pg_catalog` / `information_schema`、MySQL の `information_schema` / `mysql` / `performance_schema` / `sys`、SQLite の `sqlite_*` 系）を除いたテーブル一覧を取得しなければならない。

3.2 ビュー、マテリアライズドビュー、外部テーブル、一時テーブルは取得対象から除外しなければならない。

3.3 `--schema` フラグが指定された場合、`erdm import` は当該スキーマに属するテーブルのみを取得対象にしなければならない（PostgreSQL / MySQL のみ）。

3.4 `--schema` フラグが未指定で PostgreSQL に接続している場合、`erdm import` は `public` スキーマを既定対象としなければならない。

3.5 取得したテーブルが 0 件であった場合、`erdm import` は終了コード非ゼロで「no user tables found in schema」を標準エラーへ出力しなければならない。

3.6 取得したテーブルは、各 DBMS が返す既定順序（PostgreSQL/MySQL は `information_schema.tables` のソート順、SQLite は `sqlite_master` のソート順）を保持しなければならない。

### 4. カラム情報の取得

**目的:** 利用者として、各テーブルのカラム定義を `.erdm` の Column 構文（物理名・型・NN/U/デフォルト等）として出力したい。

#### 受け入れ条件

4.1 `erdm import` は各テーブルについて、カラムの宣言順（PostgreSQL `ordinal_position`、MySQL `ORDINAL_POSITION`、SQLite `pragma_table_info().cid`）を保持してカラム一覧を取得しなければならない。

4.2 取得した各カラムについて、`erdm import` は物理名・データ型を取得し `.erdm` の `<name> [<type>]` として出力しなければならない。

4.3 取得した各カラムが NOT NULL 制約を持つ場合、`erdm import` は `[NN]` 属性を出力しなければならない。

4.4 取得した各カラムがテーブル内で UNIQUE 制約（単一カラム UNIQUE、または単一カラムの UNIQUE INDEX）を持つ場合、`erdm import` は `[U]` 属性を出力しなければならない。

4.5 取得した各カラムにデフォルト値が設定されている場合、`erdm import` は `[=<defaultValue>]` 属性を出力しなければならない。

4.6 取得した各カラムが NULL 許容かつデフォルト値を持たない場合、`erdm import` は `[NN]` も `[=...]` も出力してはならない。

4.7 PostgreSQL の `bigserial` / `serial` / `smallserial` 型のカラム、MySQL の `AUTO_INCREMENT` 列、SQLite の `INTEGER PRIMARY KEY` 自動インクリメント列を検出した場合、`erdm import` は型表記を `bigserial` / `serial` / `int` / `bigint` 等へ正規化し、デフォルト値（`nextval(...)` 等）を `[=...]` 属性として出力してはならない。

### 5. 主キーの取得

**目的:** 利用者として、各テーブルの主キー構成カラムを `.erdm` の `+` 接頭辞付きで出力したい。

#### 受け入れ条件

5.1 `erdm import` は各テーブルの主キー制約を取得し、構成カラムの `.erdm` 行頭に `+` を付与しなければならない。

5.2 主キーが複合キーであった場合、`erdm import` は構成カラムすべての行頭に `+` を付与しなければならない。

5.3 主キーが定義されていないテーブルについて、`erdm import` はいかなるカラム行にも `+` を付与してはならない。

### 6. 外部キーの取得

**目的:** 利用者として、各テーブルの外部キーを `.erdm` のカーディナリティ記法（例: `0..*--1 <table>`）で出力したい。

#### 受け入れ条件

6.1 `erdm import` は各テーブルの外部キー制約を取得し、参照元カラムの行末に `0..*--1 <参照先テーブル名>` を付与しなければならない。

6.2 取得した外部キーの参照元カラムが NOT NULL 制約を持つ場合、`erdm import` はカーディナリティを `1..*--1 <参照先テーブル名>` として出力しなければならない。

6.3 取得した外部キーが UNIQUE 制約を伴う 1 対 1 関係であった場合、`erdm import` はカーディナリティを `0..1--1 <参照先テーブル名>` として出力しなければならない。

6.4 外部キーが複合キーであった場合、`erdm import` は構成カラムの先頭カラム行にのみ `0..*--1 <参照先テーブル名>` を付与し、残余の構成カラムには付与してはならない。

6.5 参照先テーブルが取得対象テーブル一覧に含まれていない外部キーが検出された場合、`erdm import` は当該外部キー記法を出力せず、標準エラーへ「skip foreign key referencing out-of-scope table: <参照先>」と警告を出力しなければならない。

### 7. インデックスの取得

**目的:** 利用者として、テーブル単位の補助インデックス（PK/UNIQUE 以外の通常インデックス）を `.erdm` のインデックス節として出力したい。

#### 受け入れ条件

7.1 `erdm import` は各テーブルから、主キー制約および UNIQUE 制約に紐づくインデックスを除いた補助インデックス一覧を取得しなければならない。

7.2 取得した補助インデックスについて、`erdm import` は `.erdm` のインデックス節（例: `index <indexName>(<col1>, <col2>)`）として、構成カラムの宣言順を保持して出力しなければならない。

7.3 取得した補助インデックスが UNIQUE 属性を持つ複合 UNIQUE インデックスであった場合、`erdm import` は `unique index <indexName>(<col1>, <col2>)` 形式で出力しなければならない。

### 8. 論理名（コメント）の出力

**目的:** 利用者として、DB に登録済みのテーブル/カラムコメントを `.erdm` の論理名として表示し、生成直後でも ER 図上で論理名が見える状態にしたい。

#### 受け入れ条件

8.1 PostgreSQL に接続している場合、`erdm import` は `pg_description` から各テーブル・各カラムのコメントを取得しなければならない。

8.2 MySQL に接続している場合、`erdm import` は `information_schema.tables.TABLE_COMMENT` および `information_schema.columns.COLUMN_COMMENT` を取得しなければならない。

8.3 SQLite に接続している場合、`erdm import` は CREATE TABLE 文（`sqlite_master.sql`）から `--` 行コメントを抽出可能な範囲で取得しなければならない（取得不能な場合は空とする）。

8.4 取得したテーブルコメントが空文字列でない場合、`erdm import` はそのテーブルの宣言行を `<物理名>/<論理名>` 形式で出力しなければならない。

8.5 取得したカラムコメントが空文字列でない場合、`erdm import` はそのカラム行を `<物理名>/<論理名>` 形式で出力しなければならない。

8.6 テーブルまたはカラムのコメントが空文字列または未取得である場合、`erdm import` は当該要素の論理名として物理名を採用し、`<物理名>/<物理名>` 形式で出力しなければならない。

8.7 コメント文字列に `/` または改行（`\n`、`\r`）が含まれる場合、`erdm import` は `/` を全角スラッシュ `／` に、改行を半角スペース 1 個に置換した値を論理名として出力しなければならない。

### 9. `.erdm` 出力フォーマットと整合性

**目的:** 利用者として、生成された `.erdm` をそのまま `erdm render` / `erdm serve` に流して図化・編集できることを保証したい。

#### 受け入れ条件

9.1 `erdm import` が出力する `.erdm` テキストは、`internal/parser.Parse` 関数によりエラーなく `*model.Schema` へパース可能でなければならない。

9.2 `erdm import` が出力した `.erdm` テキストをパースして得た `*model.Schema` に対し、`Schema.Validate()` を実行した結果が nil でなければならない。

9.3 `erdm import` が出力した `.erdm` テキストをパース → `internal/serializer.Serialize` で再シリアライズした結果が、再度パース可能であり、かつもう一度シリアライズしてもバイト一致しなければならない（要件 7.10 の往復冪等性継承）。

9.4 `erdm import` の出力 1 行目は `# Title: <タイトル>` でなければならない。

9.5 `--title` フラグが指定された場合、`erdm import` は当該文字列を 1 行目の `# Title:` に採用しなければならない。

9.6 `--title` フラグが未指定の場合、`erdm import` は接続先 DB 名（PostgreSQL/MySQL: 接続先 DB 名、SQLite: ファイル名のベース部）をタイトルとして採用しなければならない。

### 10. DB アクセスの安全性

**目的:** 運用者として、本機能が本番 DB に影響を与えないことを保証したい。

#### 受け入れ条件

10.1 `erdm import` は対象 DB に対して、`SELECT` および `PRAGMA`（SQLite）以外のステートメントを発行してはならない。

10.2 `erdm import` は接続後、トランザクションを開始する場合は READ ONLY モード（PostgreSQL: `SET TRANSACTION READ ONLY`、MySQL: `START TRANSACTION READ ONLY`）で開始しなければならない。

10.3 `erdm import` は処理完了時または異常終了時のいずれにおいても、確立した DB 接続を閉じなければならない。

10.4 `erdm import` は DSN 文字列をログ・標準出力・標準エラーへそのまま出力してはならない。出力が必要な場合は、パスワード相当部分を `***` でマスクしなければならない。

### 11. エラー処理と終了コード

**目的:** スクリプト連携する利用者として、成功・失敗を終了コードで判定したい。

#### 受け入れ条件

11.1 `erdm import` が `.erdm` ファイル（または標準出力）への書き出しまで成功した場合、終了コードは 0 でなければならない。

11.2 DB 接続失敗、スキーマ取得失敗、`.erdm` 書き出し失敗のいずれかが発生した場合、`erdm import` は終了コード非ゼロで終了しなければならない。

11.3 取得した `*model.Schema` が `Schema.Validate()` で違反を返した場合、`erdm import` はファイルを書き出さず、終了コード非ゼロで違反内容を標準エラーへ出力しなければならない。

11.4 `--out` で指定された出力先ディレクトリが存在しない場合、`erdm import` はファイルを書き出さず、終了コード非ゼロで「output directory not found: <パス>」を標準エラーへ出力しなければならない。

### 12. 既存パッケージとの分離

**目的:** メンテナとして、新機能を既存の render / serve から分離して保守したい。

#### 受け入れ条件

12.1 DB 接続およびスキーマ取得処理は `internal/introspect`（または同等のパッケージ）配下に配置し、`internal/parser` / `internal/serializer` / `internal/server` の既存ファイルへ DB ドライバ依存を持ち込んではならない。

12.2 `internal/introspect` が公開するインターフェースは `*model.Schema` を返す関数を中心に構成し、`internal/serializer.Serialize` を介して `.erdm` テキストへ変換できなければならない。

12.3 DB ドライバの import は `erdm.go` または `internal/introspect` 配下のみに限定し、`go.mod` への追加は postgres / mysql / sqlite 用の最小限ドライバに留めなければならない。

## 要件間の依存関係

- 要件 1（CLI）は要件 2（DSN 受理）・要件 11（終了コード）の上位構造として機能する。
- 要件 3（テーブル一覧）は要件 4（カラム）・要件 5（PK）・要件 6（FK）・要件 7（インデックス）・要件 8（コメント）の前提となる。
- 要件 6（FK）は要件 4（カラム）の取得結果に依存し、参照元カラムが必ず取得済みであることを前提とする。
- 要件 9（.erdm 整合性）は要件 4〜8 の出力すべてに対する横断的検証要件である。
- 要件 12（パッケージ分離）は要件 1〜11 の実装配置に対する横断的制約である。
