mod release

set quiet := true
set shell := ["bash", "-cu", "-o", "pipefail"]

import? 'local.just'

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

fmt:
    go fmt ./...

lint:
    go vet ./...
    golangci-lint run --color never

fix:
    golangci-lint run --color never --fix

check: test fmt lint

test:
    go test -cover ./...
