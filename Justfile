set quiet := true

import? 'local.just'

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

fmt:
    just --fmt
