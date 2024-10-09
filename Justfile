set quiet := true

import? 'local.just'

[private]
help:
    just --list --unsorted

dev:
    watchexec just install

fmt:
    just --fmt
    find . -name '*.just' -print0 | xargs -0 -I{} just --fmt -f {}
