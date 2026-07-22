VERSION ?= $(shell date +%Y.%m.%d)

# Where `make deploy` sends things. Override on the command line:
#   make deploy HOST=ubuntu@1.2.3.4
HOST ?= root@your-server-ip
INSTALL_DIR ?= /opt/autobotgo
SERVICE_USER ?= autobotgo

LDFLAGS = -s -w -X main.version=$(VERSION)
GOBUILD = go build -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all build build-linux build-linux-amd64 build-linux-arm64 \
        test vet fmt check deploy clean help

all: build

## build: compile for the current machine
build:
	$(GOBUILD) -o bin/autobotgo .

## build-linux: compile for a linux server (alias for amd64)
build-linux: build-linux-amd64

## build-linux-amd64: compile for x86_64 servers (EC2 t3, DigitalOcean droplets)
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o bin/autobotgo-linux-amd64 .

## build-linux-arm64: compile for arm64 servers (EC2 Graviton, t4g)
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -o bin/autobotgo-linux-arm64 .

## test: run the test suite
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format the source
fmt:
	gofmt -w .

## check: format check, vet, and tests
check: fmt vet test

## deploy: upload a new binary and restart the service
deploy: build-linux-amd64
	scp bin/autobotgo-linux-amd64 $(HOST):autobotgo.new
	ssh $(HOST) 'sudo install -o $(SERVICE_USER) -g $(SERVICE_USER) -m 0755 autobotgo.new $(INSTALL_DIR)/autobotgo && rm -f autobotgo.new && sudo systemctl restart autobotgo'
	@echo "deployed; watch it with: ssh $(HOST) journalctl -u autobotgo -f"

## clean: remove build output
clean:
	rm -rf bin

## help: list targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
