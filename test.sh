#!/usr/bin/env bash

set -euo pipefail

if ! hash golint 2>/dev/null; then
    go get github.com/golang/lint/golint
fi
if ! hash megacheck 2>/dev/null; then
    go get honnef.co/go/tools/cmd/megacheck
fi

echo + go fmt
gofiles="$(find . -type f -iname '*.go')"
if [ -n "$gofiles" ]; then
    unformatted="$(gofmt -l $gofiles)"
    if [ -n "$unformatted" ]; then
        echo >&2 "Go files must be formatted with gofmt. Please run:"
        for fn in $unformatted; do
            echo >&2 "  gofmt -w $PWD/$fn"
        done
        exit 1
    fi
fi

set -x
go vet ./...
golint ./...
megacheck ./...
go test -v ./...
