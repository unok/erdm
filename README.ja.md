# erdm (ERD Musou)

[English](README.md)

テキストで書いた 1 つの `.erdm` ファイルから、ER 図 (Graphviz による PNG / SVG)、閲覧用 HTML、PostgreSQL / SQLite の DDL、ELK JSON、対話的な Web UI までを生成するテキストベースの ERD ツールです。

## 概要

`erdm` はテーブル・カラム・インデックス・リレーションを記述する簡潔な DSL を読み取り、ひとつのソースから複数の成果物を生成します。

- **図**: Graphviz DOT / PNG（`rankdir=LR`、直交ルーティング `splines=ortho`）。
- **HTML**: ER 図を埋め込んだ閲覧用スキーマリファレンス。
- **DDL**: PostgreSQL / SQLite 向けの `CREATE TABLE`。
- **ELK JSON**: 同梱 Web UI や外部ツール向けのレイアウトエンジン用 JSON。
- **Web UI** (`erdm serve`): React + React Flow + elkjs フロントを単一バイナリで配信し、閲覧・手動レイアウト調整・各種エクスポートを行える HTTP サーバ。

## 必要環境

- [Go](https://go.dev/) 1.26 以上
- [Graphviz](http://www.graphviz.org/) — 既定の `--format=dot` 経路、および `erdm serve` の SVG/PNG エクスポートで `dot` コマンドが `PATH` 上にあることを要求します。`--format=elk` のみであれば不要です。
- [Node.js](https://nodejs.org/) と npm — バイナリに同梱されるフロントエンド資産をビルドするときだけ必要です。

## インストール / ビルド

```shell
# 開発ビルド（frontend/package.json が無ければフロントビルドはスキップ）
make build

# リリースビルド（フロントエンドの dist 同梱を必須化）
RELEASE=1 make build

# テスト
make test

# gox によるクロスコンパイル
make release
```

## 使い方

`erdm` は 3 つのサブコマンドを提供します。引数なしで実行すると render モードの usage 文字列が表示されます。

| サブコマンド | 用途 |
| --- | --- |
| （指定なし／既定） | `.erdm` を DOT / PNG / HTML / PostgreSQL DDL / SQLite DDL に出力。`--format=elk` 指定で ELK JSON を出力。 |
| `serve` | Web UI HTTP サーバを起動（閲覧・手動レイアウト調整・エクスポート）。 |
| `import` | 稼働中の RDBMS に接続し、現在のスキーマから `.erdm` ソースを生成。 |

フラグは `-flag` / `--flag` どちらの記法でも受理し、`=value` でも空白区切りでも指定できます。

### render モード（既定）

```text
erdm [-output_dir DIR] [--format=dot|elk] schema.erdm
```

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `-output_dir DIR` | カレントディレクトリ | 生成成果物の出力先。事前に存在している必要があります。 |
| `--format=dot\|elk` | `dot` | `dot`: DOT / PNG / HTML / `*.pg.sql` / `*.sqlite3.sql` を `-output_dir` へ生成。`elk`: ELK JSON を標準出力へ書き出し（`-output_dir` を明示指定した場合のみ `<output_dir>/<basename>.elk.json` へ書き出し）。 |

`--format=dot` は PNG 生成のため Graphviz の `dot` コマンドを `PATH` 上に要求します。`--format=elk` には不要です。

```shell
# DOT / PNG / HTML / *.pg.sql / *.sqlite3.sql を ./out へ生成
erdm -output_dir out doc/sample/test_jp.erdm

# ELK JSON を標準出力へ
erdm --format=elk doc/sample/test_jp.erdm

# ELK JSON を <output_dir>/<basename>.elk.json へ
erdm --format=elk -output_dir out doc/sample/test_jp.erdm
```

### serve モード（Web UI）

```text
erdm serve [--port=N] [--listen=ADDR] [--no-write] schema.erdm
```

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `--port=N` | `8080` | HTTP リッスンポート。 |
| `--listen=ADDR` | `127.0.0.1` | HTTP リッスンアドレス。 |
| `--no-write` | off | 読み取り専用モード。PUT 系 API は 403 を返します。 |

サーバが提供するエンドポイント:

| パス | メソッド | 用途 |
| --- | --- | --- |
| `/` | GET | SPA（React + React Flow + elkjs） |
| `/api/schema` | GET / PUT | `.erdm` ソースの取得 / 書き戻し |
| `/api/layout` | GET / PUT | `<schema>.erdm.layout.json`（手動配置座標）の取得 / 書き戻し |
| `/api/export/ddl` | GET | PostgreSQL / SQLite DDL |
| `/api/export/svg` | GET | Graphviz による SVG |
| `/api/export/png` | GET | Graphviz による PNG |

`svg` / `png` のエクスポートエンドポイントは `dot`（Graphviz）が `PATH` 上に必要で、未導入時は 503 を返します。それ以外のエンドポイントは Graphviz なしで動作します。

### import モード（稼働中 RDBMS → `.erdm`）

`erdm import` は稼働中の PostgreSQL / MySQL / SQLite に接続し、現在のスキーマから `.erdm` ソースファイルを生成します。

```text
erdm import --dsn=<DSN> [--driver=postgres|mysql|sqlite] [--out=PATH] [--title=NAME] [--schema=NAME]
```

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `--dsn=DSN` | （必須） | 取得元データベースの DSN。エラー／ログ出力時はパスワード部がマスクされます。 |
| `--driver=NAME` | DSN から自動判定 | `postgres` / `mysql` / `sqlite` を強制指定。判定ルールは下表参照。 |
| `--out=PATH` | 標準出力 | 出力先 `.erdm` ファイルパス。親ディレクトリは事前に存在している必要があります。 |
| `--title=NAME` | DB 名（PG / MySQL）／ファイル名ベース（SQLite） | 出力 `.erdm` の `# Title:` 行に書き込まれるタイトル。 |
| `--schema=NAME` | `public`（PostgreSQL）／ `SELECT DATABASE()` で解決した接続先 DB 名（MySQL） | 取得対象スキーマ名。SQLite では無視されます。 |

ドライバ自動判定（大文字小文字を区別しません）:

| DSN の形式 | 判定されるドライバ |
| --- | --- |
| `postgres://...`、`postgresql://...` | `postgres` |
| `mysql://...`、`user:pass@tcp(host:port)/db` | `mysql` |
| `file:...`、`*.db`、`*.sqlite`、`*.sqlite3` | `sqlite` |

```shell
# SQLite ファイルから .erdm を標準出力へ
erdm import --dsn=./app.db > schema.erdm

# 同じ内容をファイルへ書き出し
erdm import --dsn=./app.db --out=./schema.erdm

# PostgreSQL：スキーマ名とタイトルを明示
erdm import \
  --dsn='postgres://user:secret@host:5432/db?sslmode=disable' \
  --schema=public \
  --title=MyApp \
  --out=./schema.erdm

# MySQL の標準 DSN（`user@tcp(...)` からドライバを推定）
erdm import \
  --dsn='user:secret@tcp(127.0.0.1:3306)/shop?parseTime=true' \
  --out=./shop.erdm
```

イントロスペクション後に内部不変条件のバリデーションが走ります。バリデーション失敗時は出力ファイルを書き出しません。

## DSL 構文

### 最小例

```text
# Title: ERサンプル

users/会員
    +id/会員ID [bigserial][NN][U]
    nick_name/ニックネーム [varchar(128)][NN]
    password/パスワード [varchar(128)]
    profile/プロフィール [text]

articles/記事
    +id/記事ID [bigserial][NN][U]
    title/タイトル [varchar(256)][NN]
    contents/内容 [text][NN]
    owner_user_id/投稿者 [bigint][NN] 0..*--1 users

tags/タグ
    +id/タグID [bigserial][NN][U]
    name/タグ [varchar(256)][NN][U]

article_tags/記事タグ管理
    +id [bigserial][NN][U]
    article_id [bigint][NN] 0..*--1 articles
    tag_id [bigint][NN] 0..*--1 tags
```

#### 出力例

![ERD サンプル](doc/sample/test_jp.png)

### 記法リファレンス

- `name/"論理名"` — 物理名と省略可能な論理名（表示名）。
- `+name` — 主キー列（`*name` も可）。
- `[type]` — カラム型。例: `[varchar(128)]`、`[bigserial]`。
- `[NN]` — `NOT NULL`。
- `[U]` — `UNIQUE`。
- `[=value]` — デフォルト値。
- `[-erd]` — このカラムを ER 図に表示しない。
- `0..*--1 別テーブル` — カーディナリティ付きリレーション。図上は FK の向きに関わらず親 → 子に正規化されます。
- `index i_name (col1, col2) unique` — インデックス宣言。`unique` は省略可。
- 列の後の `# コメント` — カラム単位のコメント。

### グルーピング (`@groups[...]`)

テーブルに 1 つ以上のグループを指定できます。先頭が **primary** グループとして cluster 描画に使われ、残りは Web UI でのバッジ / 色帯 / フィルタ用ヒントとして利用されます。

```text
table user_orders @groups["Order", "User", "Billing"]
    +id [bigint][NN][U]
    user_id [bigint][NN] 0..*--1 users
    order_id [bigint][NN] 0..*--1 orders
```

`@groups` が無いテーブルは ungrouped として cluster なしで描画されます。

## リポジトリ構成

```
erdm/
├── erdm.go                 CLI エントリ（render / serve のディスパッチ）
├── internal/
│   ├── parser/             PEG ベースの .erdm パーサ（parser.peg）
│   ├── model/              スキーマ / テーブル / FK / グループの Go 構造体
│   ├── dot/                Graphviz DOT 出力
│   ├── ddl/                PostgreSQL / SQLite DDL 出力
│   ├── html/               HTML スキーマリファレンス出力
│   ├── elk/                ELK JSON 出力
│   ├── layout/             layout.json の I/O
│   └── server/             erdm serve の HTTP ハンドラ
├── frontend/               Vite + React + TS + React Flow + elkjs の SPA
│   └── dist/               ビルド成果物（embed.FS で Go バイナリへ同梱）
├── doc/sample/             サンプル .erdm 群
└── Makefile                ビルド / テスト / リリースの一連フロー
```

## ライセンス

[MIT](https://github.com/tcnksm/tool/blob/master/LICENCE)

## 作者

[unok](https://github.com/unok)
