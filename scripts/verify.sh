#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

go mod tidy
go test -race -count=1 -shuffle=on ./...
go vet ./...

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "gofmt needed:"
  echo "$unformatted"
  exit 1
fi

go build -o /tmp/weave-image .
