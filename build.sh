#!/bin/sh

rm erdm.peg.go
peg erdm.peg
rm templates_files.go
go-bindata -o=templates_files.go ./templates/...
gox -osarch "linux/amd64 darwin/amd64 windows/amd64 windows/i386" -output "bin/{{.Dir}}_{{.OS}}_{{.Arch}}"
