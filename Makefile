SHELL := /bin/bash

.PHONY: api
api:
	go run ./services/controlplane/cmd/api

.PHONY: worker
worker:
	go run ./services/controlplane/cmd/worker

.PHONY: console
console:
	pnpm --filter @evo/console dev

.PHONY: mcp
mcp:
	pnpm --filter @evo/mcp dev

.PHONY: mcp-http
mcp-http:
	pnpm --filter @evo/mcp dev:http

.PHONY: chatgpt
chatgpt:
	pnpm --filter @evo/chatgpt-app dev

.PHONY: test
test: test-go test-ts

.PHONY: test-go
test-go:
	go test ./services/controlplane/...

.PHONY: test-ts
test-ts:
	pnpm test

.PHONY: smoke-cli
smoke-cli:
	bash scripts/smoke-controlplane.sh
