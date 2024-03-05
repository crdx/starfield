set quiet

[private]
help:
    just --list --unsorted

dev:
	watchexec just make-bin

make: make-bin make-wasm

make-bin:
	go build -o bin/starfield 2>&1 | gostack --mod crdx.org/starfield

make-wasm:
    GOOS=wasip1 GOARCH=wasm go build -o bin/starfield.wasm
