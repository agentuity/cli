.PHONY: build fmt generate

build: fmt generate
	@go build -o agentuity

fmt:
	@go fmt ./...

generate:
	@echo "Running go generate..."
	@go generate ./...