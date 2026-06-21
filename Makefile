.PHONY: test test-go test-web build run

test: test-go test-web

test-go:
	go test ./...

test-web:
	cd web && npm test

build:
	cd web && npm run build
	go build -buildvcs=false -o dist/xpanel ./cmd/xpanel

run:
	XPANEL_SEED_DEMO=true go run ./cmd/xpanel
