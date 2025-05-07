.PHONY: build fmt generate test_install_linux test_install_alpine

build: fmt generate
	@go build -o agentuity

fmt:
	@go fmt ./...

generate:
	@echo "Running go generate..."
	@go generate ./...

test_install_linux:
	@docker build -t agentuity-test-install-linux -f install_test/Dockerfile-Ubuntu	 .
	@docker run -it agentuity-test-install-linux

test_install_alpine:
	@docker build -t agentuity-test-install-alpine -f install_test/Dockerfile-Alpine .
	@docker run -it agentuity-test-install-alpine

