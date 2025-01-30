.PHONY: build fmt

build: fmt
	@go build -o agentuity

fmt:
	@go fmt ./...