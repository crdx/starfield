set quiet

[private]
help:
    just --list --unsorted

dev:
	watchexec just install

make:
	go build -o bin/starfield 2>&1 | gostack --mod crdx.org/starfield

install: make
    go install
