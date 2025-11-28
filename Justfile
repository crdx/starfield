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
    unbuffer golangci-lint --color never run | gostack

fix:
    unbuffer golangci-lint --color never run --fix | gostack

test:
    unbuffer go test -cover ./... | gostack --test
