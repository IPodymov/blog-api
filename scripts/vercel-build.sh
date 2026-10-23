#!/bin/sh
set -eu

# Preview credentials must point to an isolated database, never production.
# Go's migration runner uses DATABASE_URL_UNPOOLED for session-safe DDL.
go run -buildvcs=false ./cmd/migrate
# Vercel strips .git via .vercelignore, so VCS stamping must be disabled.
go build -buildvcs=false -trimpath -ldflags '-s -w' -o "${VERCEL_OUTPUT_FILE:-server}" ./cmd/api
