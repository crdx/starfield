set quiet

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

install:
    go install 2>&1 | gostack --mod crdx.org/starfield
