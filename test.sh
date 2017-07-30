#!/usr/bin/env bash

set -xeuo pipefail

if ! hash golint 2>/dev/null; then
    go get github.com/golang/lint/golint
fi
if ! hash megacheck 2>/dev/null; then
    go get honnef.co/go/tools/cmd/megacheck
fi

go vet ./...
golint ./...
megacheck ./...
go test -v ./...
