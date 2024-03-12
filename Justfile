set quiet

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

install:
    CGO_ENABLED=0 go build -trimpath -o $HOME/Dropbox/bin/ ./cmd/starfield 2>&1 \
        | gostack --mod crdx.org/starfield
