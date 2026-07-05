#!/bin/sh
# build.sh は Makefile の `release` ターゲットを呼び出す薄いラッパ。
# 既存の利用フローを破壊しないために残してある。
# 直接 PEG 再生成・フロントビルド・Go ビルドを行いたい場合は make を直接呼ぶこと。
#
# 例:
#   make build               開発ビルド（フロント未生成でも継続）
#   RELEASE=1 make build     リリースビルド（フロント資産の同梱を必須化）
#   make release             gox を使ったクロスコンパイル（このスクリプトと同等）

set -eu

DIR="$(cd "$(dirname "$0")" && pwd)"
exec make -C "$DIR" release
