# Makefile — ELK + Web UI 移行のビルドフロー（PEG 再生成 → フロントビルド → Go ビルド）
#
# 通常開発:
#   make build                   PEG 再生成 + フロント（ある場合）+ Go ビルド
#   make test                    Go テストを実行
#   make gen                     PEG 自動生成コードのみ更新
#   make frontend                フロント側のビルドのみ実行（package.json が無ければスキップ）
#   make verify-frontend         フロント成果物（dist/index.html）の存在を確認
#   make clean                   生成物を削除
#   make release                 gox を使ったクロスコンパイル成果物を bin/ へ生成
#
# リリース時:
#   RELEASE=1 make build         フロント未生成を許容せず必ず存在を要求する
#
# 注意:
#   フロントエンド実体（frontend/）は本フィーチャー後段（tasks 7.x）で構築する。
#   それまでは frontend/package.json 不在でも build を停止させない暫定モードで動かす。

GO          ?= go
NPM         ?= npm
RELEASE     ?= 0
PKG         := ./...
PEG_PARSER_SRC := internal/parser/parser.peg
PEG_PARSER_OUT := internal/parser/parser.peg.go
FRONTEND    := frontend
FRONTEND_DIST := $(FRONTEND)/dist
FRONTEND_INDEX := $(FRONTEND_DIST)/index.html
BIN_DIR     := bin
GOX_TARGETS := linux/amd64 darwin/amd64 windows/amd64 windows/i386

.PHONY: all build gen frontend verify-frontend frontend-test test check-requirements clean release help tools

all: build

help:
	@echo "Targets: build | gen | frontend | verify-frontend | test | check-requirements | clean | release"
	@echo "Set RELEASE=1 to require frontend dist for build."

# --------------------------------------------------------------------------
# Stage 1: PEG 自動生成コードを再生成する
#   tasks 4.1 で旧 root の `erdm.peg`/`erdm.peg.go`（package main）を撤去し、
#   `internal/parser/parser.peg`/`parser.peg.go`（package parser）の単一経路に
#   集約済み。
# --------------------------------------------------------------------------
gen: $(PEG_PARSER_OUT)

$(PEG_PARSER_OUT): $(PEG_PARSER_SRC)
	@echo ">> regenerating $(PEG_PARSER_OUT) from $(PEG_PARSER_SRC)"
	@$(GO) run github.com/pointlander/peg $(PEG_PARSER_SRC)

# --------------------------------------------------------------------------
# Stage 2: フロントエンドをビルドする
#   frontend/package.json が無ければスキップ（tasks 7.1 までの暫定）。
#   RELEASE=1 のときは package.json も dist/index.html も必須。
# --------------------------------------------------------------------------
frontend:
	@if [ -f $(FRONTEND)/package.json ]; then \
		echo ">> building frontend in $(FRONTEND)"; \
		(cd $(FRONTEND) && $(NPM) ci && $(NPM) run build); \
	else \
		if [ "$(RELEASE)" = "1" ]; then \
			echo "ERROR: $(FRONTEND)/package.json not found but RELEASE=1 requires it" >&2; \
			exit 1; \
		fi; \
		echo ">> $(FRONTEND)/package.json not found; skipping frontend build (development mode)"; \
	fi

# verify-frontend は単一バイナリ配布の前提（フロント資産同梱）を担保するゲート。
# 開発時（RELEASE=0）は dist が無くても警告に留める。
# リリース時（RELEASE=1）は dist/index.html が無いと Go ビルド前で停止する。
verify-frontend:
	@if [ -f $(FRONTEND_INDEX) ]; then \
		echo ">> $(FRONTEND_INDEX) present"; \
	else \
		if [ "$(RELEASE)" = "1" ]; then \
			echo "ERROR: $(FRONTEND_INDEX) is required for release build but missing" >&2; \
			exit 1; \
		fi; \
		echo "WARN: $(FRONTEND_INDEX) not found; continuing (set RELEASE=1 to require it)"; \
	fi

# --------------------------------------------------------------------------
# Stage 3: Go ビルド
# --------------------------------------------------------------------------
build: gen frontend verify-frontend
	@echo ">> go build"
	@$(GO) build $(PKG)

# --------------------------------------------------------------------------
# フロントエンドの単体テスト (Vitest, タスク 7.8)
#   frontend/package.json が存在しなければスキップ（基盤未整備の暫定モードと
#   挙動を揃える）。本物の package.json が無い場合はメッセージのみ出して 0 終了。
# --------------------------------------------------------------------------
frontend-test:
	@if [ -f $(FRONTEND)/package.json ]; then \
		echo ">> running frontend tests in $(FRONTEND)"; \
		(cd $(FRONTEND) && $(NPM) test); \
	else \
		echo ">> $(FRONTEND)/package.json not found; skipping frontend tests"; \
	fi

test: check-requirements frontend-test
	@$(GO) test $(PKG)

# --------------------------------------------------------------------------
# 要件カバレッジ確認 (タスク 8.3)
#   design.md §要件トレーサビリティ表に記載された全要件 ID（範囲展開後）が
#   テストファイルの `Requirements:` コメントで参照されていることを検証する。
#   ドキュメント専用 ID（3.1 / 8.5 / 8.6）は除外される。
# --------------------------------------------------------------------------
check-requirements:
	@bash scripts/check-requirements-coverage.sh

clean:
	@rm -rf $(BIN_DIR) $(FRONTEND_DIST)
	@echo ">> cleaned $(BIN_DIR) and $(FRONTEND_DIST)"

# --------------------------------------------------------------------------
# クロスコンパイル成果物（旧 build.sh 相当）。gox 必須。
# --------------------------------------------------------------------------
release: gen frontend verify-frontend
	@command -v gox >/dev/null 2>&1 || { \
		echo "ERROR: gox not found in PATH; install via 'go install github.com/mitchellh/gox@latest'" >&2; \
		exit 1; \
	}
	@gox -osarch "$(GOX_TARGETS)" -output "$(BIN_DIR)/{{.Dir}}_{{.OS}}_{{.Arch}}"

# --------------------------------------------------------------------------
# tools: tools/tools.go を経由した依存解決のスモークテスト
# --------------------------------------------------------------------------
tools:
	@$(GO) build -tags tools ./tools/...
	@echo ">> tools dependencies resolved"
