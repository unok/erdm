package main

import "embed"

//go:embed templates/*.tmpl
var templatesFS embed.FS

func Asset(name string) ([]byte, error) {
	return templatesFS.ReadFile(name)
}
