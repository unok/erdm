# erdm (ERD Musou)

[日本語版](README.ja.md)

A text-based ERD tool that turns a single `.erdm` file into ER diagrams (PNG / SVG via Graphviz), browsable HTML, PostgreSQL / SQLite DDL, ELK JSON, and an interactive Web UI.

## Description

`erdm` reads a concise text DSL describing your tables, columns, indexes, and relationships and produces multiple artifacts from one source of truth:

- **Diagram**: Graphviz DOT / PNG with `rankdir=LR` and orthogonal edge routing.
- **HTML**: a browsable schema reference embedding the rendered diagram.
- **DDL**: PostgreSQL and SQLite `CREATE TABLE` statements.
- **ELK JSON**: layout-engine-ready JSON for the bundled Web UI or external tools.
- **Web UI** (`erdm serve`): a single-binary HTTP server that ships a React + React Flow + elkjs front-end for browsing, manually adjusting layout, and exporting.

## Requirements

- [Go](https://go.dev/) 1.26+
- [Graphviz](http://www.graphviz.org/) — `dot` must be on `PATH` for the default `--format=dot` render path and for SVG/PNG export from `erdm serve`. Not required for `--format=elk`.
- [Node.js](https://nodejs.org/) + npm — only needed to build the front-end assets that get embedded into the binary.

## Install / Build

```shell
# Development build (skips front-end if frontend/package.json is absent)
make build

# Release build (requires the embedded front-end bundle to be present)
RELEASE=1 make build

# Run tests
make test

# Cross-compiled release binaries via gox
make release
```

## Usage

### Render mode (default)

```shell
# Generate DOT / PNG / HTML / *.pg.sql / *.sqlite3.sql into ./out
erdm -output_dir out doc/sample/test.erdm

# Emit ELK JSON to stdout
erdm --format=elk doc/sample/test.erdm

# Emit ELK JSON to <output_dir>/<basename>.elk.json
erdm --format=elk -output_dir out doc/sample/test.erdm
```

### Serve mode (Web UI)

```shell
erdm serve [--port=8080] [--listen=127.0.0.1] [--no-write] schema.erdm
```

The server exposes:

| Path | Method | Purpose |
| --- | --- | --- |
| `/` | GET | SPA (React + React Flow + elkjs) |
| `/api/schema` | GET / PUT | Read or write back the `.erdm` source |
| `/api/layout` | GET / PUT | Read or write `<schema>.erdm.layout.json` (manual coordinates) |
| `/api/export/ddl` | GET | PostgreSQL / SQLite DDL |
| `/api/export/svg` | GET | SVG via Graphviz |
| `/api/export/png` | GET | PNG via Graphviz |

`--no-write` switches the server into read-only mode (PUT endpoints reject with 403).

## DSL Syntax

### Minimal example

```text
# Title: ER Sample

users/"site user master"
    +id/"member id" [bigserial][NN][U]
    nick_name/nickname [varchar(128)][NN]
    password/"site password" [varchar(128)]
    profile/profile [text]

articles/article
    +id/"article id" [bigserial][NN][U]
    title/"article title" [varchar(256)][NN]
    contents/"article text" [text][NN]
    owner_user_id/creator [bigint][NN] 0..*--1 users

tags/"tag master"
    +id/"tag id" [bigserial][NN][U]
    name/"tag name" [varchar(256)][NN][U]

article_tags/"article tag relation table"
    +id [bigserial][NN][U]
    article_id [bigint][NN] 0..*--1 articles
    tag_id [bigint][NN] 0..*--1 tags
```

#### Output

![ERD sample](doc/sample/test.png)

### Notation reference

- `name/"logical name"` — physical name and optional logical (display) name.
- `+name` — primary key column. `*name` is also accepted.
- `[type]` — column type, e.g. `[varchar(128)]`, `[bigserial]`.
- `[NN]` — `NOT NULL`.
- `[U]` — unique.
- `[=value]` — default value.
- `[-erd]` — hide this column from the ER diagram.
- `0..*--1 other_table` — relationship with cardinality. The diagram is drawn parent → child regardless of FK direction.
- `index i_name (col1, col2) unique` — index declaration. `unique` is optional.
- `# comment` after a column — column-level comment.

### Grouping (`@groups[...]`)

Tables can be tagged with one or more groups. The first entry is the *primary* group used for cluster rendering; remaining entries are available for badges, filters, and color hints in the Web UI.

```text
table user_orders @groups["Order", "User", "Billing"]
    +id [bigint][NN][U]
    user_id [bigint][NN] 0..*--1 users
    order_id [bigint][NN] 0..*--1 orders
```

Tables without `@groups` are rendered as ungrouped (no cluster).

## Repository layout

```
erdm/
├── erdm.go                 CLI entry point (render / serve dispatch)
├── internal/
│   ├── parser/             PEG-based .erdm parser (parser.peg)
│   ├── model/              Schema / table / FK / group structs
│   ├── dot/                Graphviz DOT renderer
│   ├── ddl/                PostgreSQL / SQLite DDL renderers
│   ├── html/               HTML schema reference renderer
│   ├── elk/                ELK JSON exporter
│   ├── layout/             layout.json I/O
│   └── server/             erdm serve HTTP handlers
├── frontend/               Vite + React + TS + React Flow + elkjs SPA
│   └── dist/               Build output, embedded into the Go binary via embed.FS
├── doc/sample/             Example .erdm files
└── Makefile                Build / test / release flow
```

## License

[MIT](https://github.com/tcnksm/tool/blob/master/LICENCE)

## Author

[unok](https://github.com/unok)
