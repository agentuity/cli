.PHONY: build fmt generate

build: fmt generate
	@go build -o agentuity

fmt:
	@go fmt ./...

generate:
	@echo "Running go generate..."
	@go generate ./...

test_install_linux:
	@docker build -t agentuity-test-install-linux -f install_test/Dockerfile .
	@docker run -it agentuity-test-install-linux

