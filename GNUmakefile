default: fmt lint install generate

# Pin golangci-lint to the version CI installs (golangci-lint-action@v6.5.1,
# which tracks the v1 line). Run it via `go run` so `make lint` matches CI
# regardless of any globally installed golangci-lint.
GOLANGCI_LINT_VERSION ?= v1.64.7

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate
