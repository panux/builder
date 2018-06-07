#!/bin/sh
set -e

echo Building "panux/$1"
docker build -f "cmd/$1/Dockerfile" -t "panux/$1" . &

if [ -e "cmd/$1/Dockerfile.alpine" ]; then
    echo Building "panux/$1:alpine"
    docker build -f "cmd/$1/Dockerfile.alpine" -t "panux/$1:alpine" . &
fi

wait
