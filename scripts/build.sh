#!/bin/sh

set -e

gobuild() {
    echo Building $1
    go build -o $1 --ldflags '-linkmode external -extldflags "-static"' github.com/panux/builder/cmd/$1
}

dbuild() {
    echo Building panux/$1
    docker build -t panux/$1 -f cmd/$1/Dockerfile .
    if [ -e cmd/$1/Dockerfile.alpine ]; then
        echo Building panux/$1:alpine
        docker build -t panux/$1:alpine -f cmd/$1/Dockerfile.alpine .
    fi
}

make -C static

for i in buildmanager dlserver pbuild worker; do
    gobuild $i
    dbuild $i
done
