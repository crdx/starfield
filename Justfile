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
    unbuffer go vet ./... | gostack
    unbuffer golangci-lint run --color never | gostack

fix:
    unbuffer golangci-lint run --color never --fix | gostack

check: test fmt lint

test:
    unbuffer go test -cover ./... | gostack --test
