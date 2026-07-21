#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
APP_NAME="poche-resend-webmail"
echo "Building ${APP_NAME}..."
go build -ldflags="-s -w" -o "${APP_NAME}" .
echo "Built: ${APP_NAME}"
ls -lh "${APP_NAME}"
