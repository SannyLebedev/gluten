#!/usr/bin/env bash

set -euo pipefail

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
