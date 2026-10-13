#!/bin/sh
set -eu

# Preview credentials must point to an isolated database, never production.
# Go's migration runner uses DATABASE_URL_UNPOOLED for session-safe DDL.
go run ./cmd/migrate
go build -trimpath -ldflags '-s -w' -o "${VERCEL_OUTPUT_FILE:-server}" ./cmd/api
