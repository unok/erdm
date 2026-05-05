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

### render モード（既定）

```shell
# DOT / PNG / HTML / *.pg.sql / *.sqlite3.sql を ./out へ生成
erdm -output_dir out doc/sample/test_jp.erdm

# ELK JSON を標準出力へ
erdm --format=elk doc/sample/test_jp.erdm

# ELK JSON を <output_dir>/<basename>.elk.json へ
erdm --format=elk -output_dir out doc/sample/test_jp.erdm
```

### serve モード（Web UI）

```shell
erdm serve [--port=8080] [--listen=127.0.0.1] [--no-write] schema.erdm
```

サーバが提供するエンドポイント:

| パス | メソッド | 用途 |
| --- | --- | --- |
| `/` | GET | SPA（React + React Flow + elkjs） |
| `/api/schema` | GET / PUT | `.erdm` ソースの取得 / 書き戻し |
| `/api/layout` | GET / PUT | `<schema>.erdm.layout.json`（手動配置座標）の取得 / 書き戻し |
| `/api/export/ddl` | GET | PostgreSQL / SQLite DDL |
| `/api/export/svg` | GET | Graphviz による SVG |
| `/api/export/png` | GET | Graphviz による PNG |

`--no-write` を指定すると読み取り専用モードになり、PUT 系 API は 403 を返します。

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
