#!/usr/bin/env bash

set -xeuo pipefail

go get -v -t -d ./...

go get github.com/golang/lint/golint
go get honnef.co/go/tools/cmd/megacheck
