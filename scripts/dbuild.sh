#!/bin/sh

echo Building "panux/$1"
exec docker build -f "cmd/$1/Dockerfile" -t "panux/$1" .
