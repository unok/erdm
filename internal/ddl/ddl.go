// Package ddl は *model.Schema から PostgreSQL / SQLite3 の DDL を生成する。
//
// 公開境界は RenderPG と RenderSQLite の 2 関数のみ。テンプレートは embed.FS で
// 同梱し、外部から参照不可な状態でパッケージ内に閉じる
// （design.md §C8「テンプレート所有」）。
//
// 出力は旧 erdm CLI が `templates/{pg,sqlite3}_ddl.tmpl` から生成していた DDL
// （DROP TABLE IF EXISTS、CREATE TABLE、PRIMARY KEY、CREATE INDEX、UNIQUE/NOT
// NULL/DEFAULT）と同等の構造を持ち、要件 9.1 / 5.6 / 8.1 / 8.2 の後方互換と
// HTTP `/api/export/ddl?dialect=...` の依存先を担保する。
package ddl

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"text/template"

	"github.com/unok/erdm/internal/model"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// RenderPG は Schema を PostgreSQL 形式の DDL へレンダリングして返す。
//
// 出力には DROP TABLE IF EXISTS ... CASCADE、CREATE TABLE、PRIMARY KEY、
// UNIQUE / NOT NULL / DEFAULT、CREATE [UNIQUE] INDEX を含む。
func RenderPG(s *model.Schema) ([]byte, error) {
	return renderTemplate(s, "pg_ddl")
}

// RenderSQLite は Schema を SQLite3 形式の DDL へレンダリングして返す。
//
// 出力には DROP TABLE IF EXISTS（CASCADE なし）、CREATE TABLE、PRIMARY KEY、
// UNIQUE / NOT NULL / DEFAULT、CREATE [UNIQUE] INDEX を含む。
func RenderSQLite(s *model.Schema) ([]byte, error) {
	return renderTemplate(s, "sqlite3_ddl")
}

// renderTemplate は s と name を受け取って指定テンプレートを実行する内部ヘルパ。
// PG / SQLite で共通の解析・実行・エラーラップ処理を一元化する。
func renderTemplate(s *model.Schema, name string) ([]byte, error) {
	if s == nil {
		return nil, errors.New("ddl: schema is nil")
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("ddl: parse templates: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, s); err != nil {
		return nil, fmt.Errorf("ddl: execute template %q: %w", name, err)
	}
	return buf.Bytes(), nil
}
