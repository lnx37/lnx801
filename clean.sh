#!/bin/bash

set -e
set -o pipefail
set -u
set -x

cd "$(dirname "$0")"

date

# rm -rf build
# rm -rf lib/go-sqlite3
# rm -f lnxmon.db
rm -rf build

date
