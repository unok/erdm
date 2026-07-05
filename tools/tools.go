//go:build tools

// Package tools は PEG パーサ生成器 github.com/pointlander/peg を
// モジュールのツール依存として固定する目的で存在する。
//
// 通常ビルドでは `tools` ビルドタグが付かないため、このファイルは
// コンパイル対象に含まれない。tools 関連の依存は `go.mod` に記録される
// ため、ローカル／CI ともに `go run github.com/pointlander/peg` で
// 同じバージョンを取得できる。
//
// PEG 再生成は Makefile の `gen` ターゲットから呼び出す。
package tools

// peg は main パッケージのコマンドのため直接 import できない。
// 同モジュール配下の importable パッケージ tree を blank import する
// ことで go mod が peg モジュールごと依存として保持し続ける。
// 実行は `go run github.com/pointlander/peg` で行う。
import (
	_ "github.com/pointlander/peg/tree"
)
