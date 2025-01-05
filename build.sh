#!/bin/bash

set -e
set -o pipefail
set -u
set -x

cd "$(dirname "$0")"

date

[ -d build ] && rm -rf build

# apt-get install gcc glibc-static
# yum install gcc glibc-static
# yum install binutils-devel
GO111MODULE=off go build -ldflags="-s -w -linkmode=external -extldflags=-static" -o build/lnx801srv lnx801srv.go
GO111MODULE=off go build -ldflags="-s -w -linkmode=external -extldflags=-static" -o build/lnx801cli lnx801cli.go

date
