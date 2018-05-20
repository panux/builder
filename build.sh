#!/bin/sh
set -e

for i in dlserver worker buildmanager pbuild; do
    scripts/dbuild.sh "$i"
done

echo Build done!
