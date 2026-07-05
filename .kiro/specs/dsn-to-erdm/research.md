# Research Log — DSN 指定による erdm ファイル生成機能

本ファイルは設計判断の根拠となる調査結果（既存実装の確認・外部依存候補・技術選定の判定）を記録する。設計本体は `design.md` を参照。

## 1. 既存実装の確認

### 1.1 CLI エントリポイント（erdm.go）

| 項目 | 結果 | 根拠 |
|------|------|------|
| サブコマンド分岐 | `render`（既定）/ `serve` の 2 系統のみ | erdm.go:46-58 |
| import 経路の有無 | なし | erdm.go 全行に `import` サブコマンド分岐なし |
| 引数解析方式 | `flag.NewFlagSet("render"/"serve", ContinueOnError)` | erdm.go:65, 221 |
| 終了コード扱い | `main` で error を `os.Exit(1)` に変換、関数は error 返却 | erdm.go:48-51, 54-57 |
| usage 文字列 | 関数外定数（`usageRender` / `usageServe`） | erdm.go:42-43 |

判定: **import サブコマンドを `args[0] == "import"` で分岐させる新ハンドラ `runImport` を追加する**のが既存パターンと整合。`runRender` / `runServe` は変更しない（要件 1.7）。

### 1.2 ドメインモデル

| 型 | 主要フィールド | 関連要件 |
|----|---------------|---------|
| `model.Schema` | `Title string`, `Tables []Table`, `Groups []string` | 9.4-9.6 |
| `model.Table` | `Name`, `LogicalName`, `Columns`, `PrimaryKeys []int`, `Indexes []Index`, `Groups []string` | 5.1-5.3, 7.1-7.3, 8.4-8.7 |
| `model.Column` | `Name`, `LogicalName`, `Type`, `AllowNull`, `IsUnique`, `IsPrimaryKey`, `Default`, `FK *FK`, `Comments []string` | 4.2-4.7, 5.1, 6.1-6.4, 8.5-8.7 |
| `model.FK` | `TargetTable`, `CardinalitySource`, `CardinalityDestination` | 6.1-6.5 |
| `model.Index` | `Name`, `Columns []string`, `IsUnique` | 7.2, 7.3 |

判定: **本機能は `model.Schema` を構築し、既存 `serializer.Serialize` でテキスト化する**経路に乗せれば要件 9.1-9.3（往復冪等性）は serializer 側の保証に委譲できる。新たに `.erdm` テキストを文字列連結で組み立てる必要はない。

### 1.3 パーサ・シリアライザ受理仕様

`internal/parser/parser.peg` および `internal/serializer/format.go` を確認した結果、本機能で生成する `.erdm` テキストは下記文法に従う必要がある。

| 要素 | 文法・正規化規則 | 出典 |
|------|----------------|------|
| Title 行 | `# Title: <Title>\n` | format.go:17-21 |
| テーブル宣言 | `<Name>[/<LogicalName>][ @groups[...]]\n` | format.go:38-49 |
| カラム行 | `    [+]<name>[/<logical>] [<type>][NN][U][=<default>][-erd][ <fk>]\n` | format.go:70-94 |
| 属性順序 | `[NN]` → `[U]` → `[=...]` → `[-erd]`（固定） | format.go:97-112 |
| FK 表記 | `<src>--<dst> <target>`（カーディナリティ両端任意、target 必須） | format.go:119-125 |
| Index 行 | `    index <name> (<col1>, <col2>, ...)[ unique]\n` | format.go:130-146 |
| 論理名引用要否 | スペース・タブ・改行・`/` を含む場合は `"..."` で囲む | format.go:152-162 |
| カーディナリティ受理パターン | `[01*](\.\.[01*])?`（`0`, `1`, `*`, `0..1`, `1..*`, `0..*` 等） | parser.peg:48 |

判定: **要件 6.1-6.3 が指定する `0..*--1` / `1..*--1` / `0..1--1` はすべて `cardinality` 文法 `[01*](..[01*])?` で受理可能**。新規文法追加は不要。

### 1.4 既存 CLI テストパターン

| 観点 | 内容 | 出典 |
|------|------|------|
| 単体テスト | 関数 `runRender` / `runServe` を直接呼び出し、tempdir に出力させる | cmd_test.go:22-56 |
| サブコマンド分岐テスト | `os.Args` 操作ではなく関数呼び出しでの分岐確認 | cmd_test.go 全体 |
| サンプル参照 | `internal/testutil/fixtures` 経由で `doc/sample/` を共有 | fixtures.go:1-53 |

判定: **`runImport` は関数として独立させ、引数 `args []string` を受け取り error を返す形にする**。テストは関数直接呼び出し + tempdir 出力検証で行う。

### 1.5 go.mod の現状

- Go 1.26.1
- 直接依存: `github.com/pointlander/peg v1.0.1` のみ
- DB ドライバ依存: なし
- 間接依存: peg ジェネレータ用 2 件のみ

判定: **DB ドライバを新規追加する必要がある**（要件 12.3）。候補は §2 で評価。

### 1.6 内部パッケージ配置パターン

`internal/` 配下は責務別ディレクトリで分割されており（parser / serializer / model / ddl / dot / elk / html / server / layout / testutil）、新機能は `internal/introspect/` を新設するのが整合。

## 2. 外部ドキュメント・依存候補

### 2.1 PostgreSQL ドライバ候補

| 候補 | 特徴 | 採否 |
|------|------|------|
| `github.com/jackc/pgx/v5` (pgx) | ネイティブプロトコル、`database/sql` 互換ドライバ `pgx/v5/stdlib` 同梱、active-maintained | **採用** |
| `github.com/lib/pq` | 旧来の純正ドライバ。`lib/pq` は新規開発を推奨されない | 不採用 |

判定根拠: pgx は `pgx/v5/stdlib` をブランクインポートすることで `database/sql.Open("pgx", dsn)` が使え、`postgres://` URL スキームをそのまま受理する（要件 2.1）。情報スキーマ取得は標準的な `SELECT` 文で実現できる。

### 2.2 MySQL ドライバ候補

| 候補 | 特徴 | 採否 |
|------|------|------|
| `github.com/go-sql-driver/mysql` | Go 標準的に使われる純 Go 実装。`user:pass@tcp(host:port)/db` DSN 表記の発祥元 | **採用** |

判定根拠: 要件 2.2 が指定する DSN 表記は本ドライバの標準形式そのもの。`database/sql.Open("mysql", dsn)` で接続可能。`information_schema` 経由で全要件を満たす情報が取得可能。

### 2.3 SQLite ドライバ候補

| 候補 | 特徴 | 採否 |
|------|------|------|
| `modernc.org/sqlite` | 純 Go 実装。CGO 不要、クロスコンパイル容易 | **採用** |
| `github.com/mattn/go-sqlite3` | C ライブラリラッパ。CGO 必須 | 不採用 |

判定根拠: erdm はクロスコンパイル成果物（`gox` ターゲット linux/darwin/windows）を出力する設計（Makefile:29）であり、CGO 不要な純 Go ドライバが望ましい。`modernc.org/sqlite` は `database/sql.Open("sqlite", "file:...")` または `database/sql.Open("sqlite", "/path/to/file.db")` 双方を受理する。

### 2.4 メタデータ取得クエリ（標準仕様準拠）

| 観点 | PostgreSQL | MySQL | SQLite |
|------|-----------|-------|--------|
| テーブル一覧 | `information_schema.tables WHERE table_schema=$1 AND table_type='BASE TABLE'` | `information_schema.tables WHERE table_schema=? AND table_type='BASE TABLE'` | `sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'` |
| カラム情報 | `information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position` | `information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ORDINAL_POSITION` | `pragma_table_info('<table>')` ORDER BY cid |
| 主キー | `information_schema.table_constraints` + `key_column_usage` | 同左 | `pragma_table_info` の `pk` 列（>0） |
| 外部キー | `information_schema.referential_constraints` + `key_column_usage` | 同左 | `pragma_foreign_key_list('<table>')` |
| インデックス | `pg_indexes` + `pg_index` 結合 / または `information_schema.statistics` 風カスタム | `information_schema.statistics WHERE table_schema=? AND table_name=?` | `pragma_index_list('<table>')` + `pragma_index_info('<index>')` |
| テーブルコメント | `pg_description` JOIN `pg_class` (relkind='r') | `information_schema.tables.TABLE_COMMENT` | `sqlite_master.sql` 文字列の `--` 行抽出 |
| カラムコメント | `pg_description` JOIN `pg_attribute` | `information_schema.columns.COLUMN_COMMENT` | 取得不能（空） |

判定: 全要件 3-8 は標準 SQL / 標準 PRAGMA で取得可能。ベンダー固有 API（pgx の `pgconn` 等）は不要のため、`database/sql` 抽象を全ドライバで共通利用する。

### 2.5 READ ONLY トランザクション制御

| DBMS | 文法 | 出典 |
|------|------|------|
| PostgreSQL | `SET TRANSACTION READ ONLY`（トランザクション開始後）または `BEGIN READ ONLY` | PostgreSQL Docs |
| MySQL | `START TRANSACTION READ ONLY`（5.6.5+） | MySQL Reference Manual |
| SQLite | 専用文なし。読み取り専用接続は `?mode=ro` クエリ、または `BEGIN DEFERRED` + SELECT のみ発行 | SQLite Docs |

判定: SQLite は要件 10.2 が「PostgreSQL/MySQL は READ ONLY トランザクションで開始」を要求しているのみで、SQLite に対する READ ONLY 強制は要件 10.1（SELECT/PRAGMA 限定）で担保する。ドライバ DSN に `?mode=ro` を強制注入する必要は要件にない。

### 2.6 DSN パスワードマスク

| DBMS | DSN 形態 | パスワード位置 |
|------|---------|---------------|
| PostgreSQL | URL `postgres://user:pass@host:port/db?...` | URL の `userinfo` 部の password 要素 |
| MySQL | `user:pass@tcp(host:port)/db?...` | 先頭 `user:pass` の `:` 区切り |
| SQLite | ファイルパス | 通常パスワードは含まれない（パスワード ATTACH は本機能スコープ外） |

判定: PostgreSQL は `net/url.URL.User.Password()` で抽出して `***` 置換可能。MySQL は go-sql-driver/mysql が公開する `mysql.ParseDSN(dsn)` で `Config.Passwd` を取得・マスクできる。SQLite は基本的に置換不要だが、ファイル名が機密の可能性に配慮し、要件 10.4 の方針として「`Config` 構造体を経由しない場合は DSN を**そのまま**ログ出力しない」を遵守する。

## 3. アーキテクチャ判断

### 3.1 パッケージ配置（要件 12 準拠）

```
internal/introspect/
    introspect.go       (公開 API: Introspect(ctx, opts) (*model.Schema, error))
    options.go          (Options 構造体: Driver/DSN/Schema/Title)
    driver.go           (Driver enum: postgres/mysql/sqlite + DSN 推定)
    postgres.go         (PostgreSQL 専用 introspector)
    mysql.go            (MySQL 専用 introspector)
    sqlite.go           (SQLite 専用 introspector)
    types.go            (内部一時 DTO: rawTable/rawColumn/rawForeignKey/rawIndex)
    builder.go          (raw DTO → *model.Schema 変換: 論理名フォールバック・FK カーディナリティ決定)
    mask.go             (DSN パスワードマスク)
```

判定根拠:
- 1 ファイル 200-400 行ガイドラインに収まるよう、ドライバごと + 共通変換ロジックで分割。
- `database/sql` 経由で各ドライバのクエリを分離し、共通の `rawTable/rawColumn/...` DTO に集約する設計により「論理名フォールバック」「FK カーディナリティ決定」「重複インデックス除外」などの**ドライバ非依存ロジック**を 1 箇所（`builder.go`）に集約できる（要件知識「操作の一覧性」）。
- 公開 API は `Introspect` 関数 1 本のみ（要件知識「パブリック API の公開範囲」）。

### 3.2 erdm.go との結合

`erdm.go` は import サブコマンドの**配線**のみを担当する：

1. 引数解析（`--driver` `--dsn` `--out` `--title` `--schema`）
2. `introspect.Introspect(ctx, options)` 呼び出し
3. `Schema.Validate()` 実行（要件 9.2 / 11.3）
4. `serializer.Serialize(schema)` 実行（要件 9.3）
5. 出力先振り分け（`--out` 指定時はファイル、未指定時は標準出力）

`erdm.go` は DB ドライバの blank import（`_ "github.com/jackc/pgx/v5/stdlib"` 等）を行うが、SQL 文や接続コードは持たない（要件 12.3）。

### 3.3 ドライバ選択方式

`internal/introspect/driver.go` で `DetectDriver(dsn string, override string) (Driver, error)` を提供：

| 入力 | 結果 |
|------|------|
| `--driver=postgres` | `Postgres` |
| `--driver=mysql` | `MySQL` |
| `--driver=sqlite` | `SQLite` |
| `--driver` 未指定 + DSN `postgres://` または `postgresql://` プレフィックス | `Postgres` |
| `--driver` 未指定 + DSN `mysql://` プレフィックス | `MySQL` |
| `--driver` 未指定 + DSN `file:` プレフィックスまたは `.db`/`.sqlite`/`.sqlite3` 拡張子 | `SQLite` |
| その他 | `unsupported driver: <値>` または `cannot infer driver from DSN` |

### 3.4 FK カーディナリティ決定ロジック

要件 6.1-6.3 を共通変換層（`builder.go`）に集約：

```
入力: 参照元カラムの NN フラグ, U フラグ（単一カラム UNIQUE 制約 or 単一カラム UNIQUE INDEX）
出力: CardinalitySource, CardinalityDestination

if (FK が複合キー) {
    先頭カラムのみに FK を付与し、6.2/6.3 のルールはまず複合の先頭カラム属性を見る
}
if (NN かつ U)            → "0..1" / "1"        # 6.3: 1 対 1
else if (NN)               → "1..*" / "1"       # 6.2
else                       → "0..*" / "1"       # 6.1（NULL 許容）
```

注: 要件 6.3 の「UNIQUE 制約を伴う 1 対 1 関係」は単一カラム外部キーで参照元カラムが UNIQUE である場合に適用。複合外部キーで複合 UNIQUE が掛かっている場合の扱いはスコープ外（6.4 が「先頭カラムのみに `0..*--1` を付与」と指定しているため、複合外部キーは常に `0..*--1` または `1..*--1` を採用する）。

### 3.5 論理名フォールバック（要件 8.6 / 8.7）

```
LogicalName(physical, comment string) string {
    if comment == "" { return physical }
    return sanitize(comment)  // "/" → "／", \n/\r → " "
}
```

`builder.go` の単一関数に集約。テーブル・カラム・両方に同一適用。

### 3.6 自動増分型の正規化（要件 4.7）

| DBMS | 検出条件 | 正規化結果 | デフォルト値の扱い |
|------|---------|-----------|-------------------|
| PostgreSQL | `column_default` が `nextval(...)` で開始 + `data_type` が integer 系 | `data_type` に応じ `smallserial`/`serial`/`bigserial` を採用 | 出力しない（`[=...]` を抑止） |
| MySQL | `extra` 列に `auto_increment` を含む | 元の `data_type` を採用（`int`/`bigint`） | 出力しない |
| SQLite | `pragma_table_info` の `type` が `INTEGER` かつ `pk=1` かつ単一主キー | 元の `INTEGER`/`integer` を採用 | 出力しない |

### 3.7 SQLite コメント抽出（要件 8.3）

`sqlite_master.sql` 列に格納された CREATE TABLE 文のテキストをパースする。簡易方針として「行末コメント `-- xxx` を抽出するが、CREATE TABLE 句末尾の単一行コメントを取得対象にしない」のは実装難度が高いため、**テーブル直前の行コメントとカラム宣言行末の `-- xxx` のみ**を取得対象とする。取得不能な場合は空文字列で返す（要件 8.3 が「取得不能な場合は空とする」を許容）。

具体実装: 正規表現 `(?m)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\b[^,\)]*?--\s*(.+?)\s*$` で「カラム名 + 行コメント」の対応を抽出する。テーブルコメントは `CREATE TABLE` の直前/直後行の `-- xxx` を採用。

注: SQLite のスキーマからコメントを完全に抽出することは原理的に不可能（コメントは構文木に保持されない）であり、要件 8.3 もそれを織り込んで「可能な範囲で」と規定している。

## 4. リスク・前提

| リスク | 影響 | 緩和策 |
|--------|------|-------|
| ドライバ追加で `go.mod` が肥大化 | バイナリサイズ増 | 3 ドライバを最小構成に限定（要件 12.3） |
| クロスコンパイル時の SQLite CGO | gox ビルド失敗 | `modernc.org/sqlite`（純 Go）を選定（§2.3） |
| 大規模 DB のメタデータ取得時間 | 数秒〜十数秒の応答遅延 | 全テーブルを 1 トランザクション内 SELECT で取得（接続コスト低減） |
| パスワード DSN のログ漏洩 | 機密情報露出 | エラー文言生成を `mask.go` 一箇所に集約（要件 10.4） |
| MySQL の `information_schema.statistics` で重複行 | 同インデックスが複数行に分かれる | `INDEX_NAME, SEQ_IN_INDEX` で集約 |
| PostgreSQL の `bigserial`/`serial` 検出漏れ | 不要な `[=nextval(...)]` 出力 | `column_default LIKE 'nextval(%'` + `data_type IN ('smallint','integer','bigint')` の AND 条件 |

## 5. 工数・影響範囲見積もり

| 領域 | 規模 | 内訳 |
|------|------|------|
| `internal/introspect/` 新設 | 約 800 〜 1100 行（テスト除く） | postgres.go ≒ 200 行 / mysql.go ≒ 200 行 / sqlite.go ≒ 250 行 / builder.go ≒ 200 行 / その他 ≒ 100 行 |
| `erdm.go` 改修 | 約 80 行追加 | `runImport` 新設、main 分岐に 1 行追加 |
| `go.mod` 追加 | 3 ドライバ + 推移依存 | pgx/v5, go-sql-driver/mysql, modernc.org/sqlite |
| 単体テスト | 約 600 行 | builder.go の純粋関数 + Driver 推定 + マスク + DSN 検証 |
| 統合テスト | 約 400 行 | postgres/mysql/sqlite それぞれ docker-compose または `testcontainers-go` で起動。**重い統合テストは別タスクで段取り** |
| ドキュメント | README 追記（CLI usage） | 約 30 行 |

ギャップ分析が未生成（gap-analysis.md なし）であるため、本見積もりは要件定義時のスコープに基づく。

## 6. 参照資料

- 既存実装:
  - `erdm.go`（CLI エントリポイント）
  - `internal/parser/parser.peg`（受理文法）
  - `internal/serializer/format.go`（テキスト書式）
  - `internal/model/{schema,table,column,fk,index}.go`（ドメインモデル）
  - `doc/sample/test_jp.erdm` / `validation_full.erdm`（出力例）
- 外部参照:
  - PostgreSQL Information Schema 仕様
  - MySQL Reference Manual: `information_schema.tables/columns/statistics`
  - SQLite PRAGMA Statements: `table_info`, `foreign_key_list`, `index_list`, `index_info`
  - `pgx/v5/stdlib` パッケージドキュメント
  - `go-sql-driver/mysql` README（DSN 文法）
  - `modernc.org/sqlite` README（DSN 受理形式）
