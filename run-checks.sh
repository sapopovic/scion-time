#!/usr/bin/env bash
set -Eeuo pipefail

# To install staticcheck see https://staticcheck.dev/docs/getting-started/#installation
# To install golangci-lint see https://golangci-lint.run/welcome/install/
go install golang.org/x/vuln/cmd/govulncheck@latest

env GOOS=linux GOARCH=amd64 go vet ./...
env GOOS=linux GOARCH=amd64 govulncheck ./...
env GOOS=linux GOARCH=amd64 staticcheck -checks "all,-ST1000" ./...
env GOOS=linux GOARCH=amd64 golangci-lint run
