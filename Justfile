set quiet

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

install:
    CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o $HOME/Dropbox/bin/ 2>&1 | gostack --mod crdx.org/starfield
