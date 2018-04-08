#!/bin/sh

echo Building "panux/$1"
exec docker build -q -f "cmd/$1/Dockerfile" -t "panux/$1" . > /dev/null
