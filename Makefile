.PHONY: build fmt lint generate test_install_linux test_install_alpine

build: fmt generate
	@go build -o agentuity

fmt:
	@go fmt ./...

lint:
	@$(MAKE) fmt
	@go mod tidy

generate:
	@echo "Running go generate..."
	@go generate ./...

test_install_linux:
	@docker build -t agentuity-test-install-linux -f install_test/Dockerfile-Ubuntu	 .
	@docker run -it agentuity-test-install-linux

test_install_alpine:
	@docker build -t agentuity-test-install-alpine -f install_test/Dockerfile-Alpine .
	@docker run -it agentuity-test-install-alpine

test_install_debian:
	@docker build -t agentuity-test-install-debian -f install_test/Dockerfile-Debian .
	@docker run -it agentuity-test-install-debian
