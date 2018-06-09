#!/bin/sh
set -e

docker build -t panux/builderbuilder -f scripts/Dockerfile .
docker run -it -v /var/run/docker.sock:/var/run/docker.sock panux/builderbuilder
