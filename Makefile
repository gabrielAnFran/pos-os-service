.PHONY: build test test-integration lint cover docker helm-lint run

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -tags integration ./tests/integration/...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, falling back to go vet + gofmt"; \
		go vet ./...; \
		test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1); \
	fi

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

docker:
	docker build -t pos-os-service:server --build-arg TARGET=server .
	docker build -t pos-os-service:outbox-dispatcher --build-arg TARGET=outbox-dispatcher .
	docker build -t pos-os-service:worker --build-arg TARGET=worker .

helm-lint:
	@if command -v helm >/dev/null 2>&1; then \
		helm lint charts/os-service; \
	else \
		echo "helm not installed, skipping lint"; \
	fi

run:
	OS_PORT=$${OS_PORT:-8081} \
	OS_DB_DSN=$${OS_DB_DSN:-"host=localhost user=postgres password=postgres dbname=os_service port=5432 sslmode=disable"} \
	OS_AMQP_URL=$${OS_AMQP_URL:-"amqp://guest:guest@localhost:5672/"} \
	go run ./cmd/server
