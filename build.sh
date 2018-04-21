#!/bin/sh
set -e

for i in dlserver worker buildmanager; do
    scripts/dbuild.sh "$i"
done
