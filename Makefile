.PHONY: build lint generate test test_install test_install_linux test_install_alpine test_install_debian

build: lint generate
	@go build -o agentuity

lint:
	@go fmt ./...
	@go vet ./...
	@go mod tidy

lint:
	@$(MAKE) fmt
	@go mod tidy

generate:
	@echo "Running go generate..."
	@go generate ./...

test_install_linux:
	@docker build -t agentuity-test-install-linux -f install_test/Dockerfile-Ubuntu	 .
	@docker run -it --rm agentuity-test-install-linux

test_install_alpine:
	@docker build -t agentuity-test-install-alpine -f install_test/Dockerfile-Alpine .
	@docker run -it --rm agentuity-test-install-alpine

test_install_debian:
	@docker build -t agentuity-test-install-debian -f install_test/Dockerfile-Debian .
	@docker run -it --rm agentuity-test-install-debian

test:
	@go test -race ./...

test_install: test_install_linux test_install_alpine test_install_debian
