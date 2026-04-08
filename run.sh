#!/bin/sh
set -e
go build ./...
go run ./cmd/server/
