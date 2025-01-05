#!/bin/bash

set -e
set -o pipefail
set -u
set -x

cd "$(dirname "$0")"

date

cd /opt/lnx801 && nohup ./lnx801srv >>lnx801srv.log 2>&1 &
cd /opt/lnx801 && nohup ./lnx801cli >>lnx801cli.log 2>&1 &

date
