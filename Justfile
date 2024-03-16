set quiet := true

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

install:
    #!/bin/bash
    set -eo pipefail
    export CGO_ENABLED=0
    unbuffer go build -trimpath -o $HOME/Dropbox/bin/ ./cmd/starfield | gostack

fmt:
    just --fmt
