#!/bin/sh
set -e

for i in dlserver worker; do
    scripts/dbuild.sh "$i"
done
