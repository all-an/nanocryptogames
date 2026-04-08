@echo off
go build ./...
if %errorlevel% neq 0 exit /b %errorlevel%
go run ./cmd/server/
