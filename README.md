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

`erdm` provides three subcommands. Run `erdm` with no arguments to print the render-mode usage line.

| Subcommand | Purpose |
| --- | --- |
| (default, no subcommand) | Render `.erdm` to DOT / PNG / HTML / PostgreSQL DDL / SQLite DDL, or to ELK JSON with `--format=elk`. |
| `serve` | Start the Web UI HTTP server (browse, edit layout, export). |
| `import` | Connect to a running RDBMS and emit a `.erdm` source file from its live schema. |

Flags accept both `-flag` and `--flag`; both `=value` and space-separated values work.

### Render mode (default)

```text
erdm [-output_dir DIR] [--format=dot|elk] schema.erdm
```

| Flag | Default | Description |
| --- | --- | --- |
| `-output_dir DIR` | current directory | Output directory for generated artifacts. Must already exist. |
| `--format=dot\|elk` | `dot` | `dot` writes DOT / PNG / HTML / `*.pg.sql` / `*.sqlite3.sql` into `-output_dir`. `elk` writes ELK JSON to stdout, or to `<output_dir>/<basename>.elk.json` when `-output_dir` is explicitly given. |

`--format=dot` requires the `dot` command (Graphviz) on `PATH` for the PNG step. `--format=elk` does not.

```shell
# Generate DOT / PNG / HTML / *.pg.sql / *.sqlite3.sql into ./out
erdm -output_dir out doc/sample/test.erdm

# Emit ELK JSON to stdout
erdm --format=elk doc/sample/test.erdm

# Emit ELK JSON to <output_dir>/<basename>.elk.json
erdm --format=elk -output_dir out doc/sample/test.erdm
```

### Serve mode (Web UI)

```text
erdm serve [--port=N] [--listen=ADDR] [--no-write] schema.erdm
```

| Flag | Default | Description |
| --- | --- | --- |
| `--port=N` | `8080` | HTTP listen port. |
| `--listen=ADDR` | `127.0.0.1` | HTTP listen address. |
| `--no-write` | off | Read-only mode. `PUT` endpoints reject with 403. |

The server exposes:

| Path | Method | Purpose |
| --- | --- | --- |
| `/` | GET | SPA (React + React Flow + elkjs) |
| `/api/schema` | GET / PUT | Read or write back the `.erdm` source |
| `/api/layout` | GET / PUT | Read or write `<schema>.erdm.layout.json` (manual coordinates) |
| `/api/export/ddl` | GET | PostgreSQL / SQLite DDL |
| `/api/export/svg` | GET | SVG via Graphviz |
| `/api/export/png` | GET | PNG via Graphviz |

The `svg` / `png` export endpoints require `dot` (Graphviz) on `PATH`; without it they respond with 503. The other endpoints work without Graphviz.

### Import mode (live RDBMS → `.erdm`)

`erdm import` connects to a running PostgreSQL, MySQL, or SQLite database and emits an `.erdm` source file from its current schema.

```text
erdm import --dsn=<DSN> [--driver=postgres|mysql|sqlite] [--out=PATH] [--title=NAME] [--schema=NAME] [--no-infer-fk]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--dsn=DSN` | (required) | Source database DSN. Passwords are masked in any error or log output. |
| `--driver=NAME` | auto-detect from DSN | Force `postgres`, `mysql`, or `sqlite`. Detection rules below. |
| `--out=PATH` | stdout | Output `.erdm` file path. The parent directory must already exist. |
| `--title=NAME` | DB name (PG / MySQL) or file base (SQLite) | Title written to the resulting `.erdm` (`# Title:` line). |
| `--schema=NAME` | `public` (PostgreSQL) / connected DB via `SELECT DATABASE()` (MySQL) | Target schema name. Ignored for SQLite. |
| `--no-infer-fk` | off (inference enabled) | Disable naming-convention FK inference. By default, columns ending with `_id` whose stripped base pluralizes to an existing table name (e.g. `tenant_id` → `tenants`, `system_agency_id` → `system_agencies`, `system_media_id` → `system_media`) get an inferred FK. Pluralization uses [jinzhu/inflection](https://github.com/jinzhu/inflection) so irregulars and uncountables (`person` → `people`, `media`, `data`) work. Explicit FK constraints from the database always take precedence; self-references and bare `id` columns are skipped. |

Driver auto-detection (case-insensitive):

| DSN form | Detected driver |
| --- | --- |
| `postgres://...`, `postgresql://...` | `postgres` |
| `mysql://...`, `user:pass@tcp(host:port)/db` | `mysql` |
| `file:...`, `*.db`, `*.sqlite`, `*.sqlite3` | `sqlite` |

```shell
# SQLite file → .erdm on stdout
erdm import --dsn=./app.db > schema.erdm

# Same, written to a file
erdm import --dsn=./app.db --out=./schema.erdm

# PostgreSQL with an explicit schema and title
erdm import \
  --dsn='postgres://user:secret@host:5432/db?sslmode=disable' \
  --schema=public \
  --title=MyApp \
  --out=./schema.erdm

# MySQL standard DSN (driver inferred from `user@tcp(...)`)
erdm import \
  --dsn='user:secret@tcp(127.0.0.1:3306)/shop?parseTime=true' \
  --out=./shop.erdm
```

The schema is validated after introspection; on validation failure no output file is written.

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
