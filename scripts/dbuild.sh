#!/bin/sh

exec docker build -f "cmd/$1/Dockerfile" -t "panux/$1" .
