VERSION ?= 0.0.2
BINARY  := agctl
GOFLAGS := -ldflags "-s -w -X github.com/airgapops/agctl/cmd.Version=$(VERSION)"

# Control Plane deployment (update with your credentials)
CP_HOST := <CP_USER>@<CP_IP>

.PHONY: build build-linux install clean deploy test

## Build for current OS
build:
	go build $(GOFLAGS) -o bin/$(BINARY) .

## Cross-compile for Linux (Control Plane)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o bin/$(BINARY)-linux .

## Install locally
install: build
	sudo cp bin/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "✅ $(BINARY) v$(VERSION) installed"

## Deploy to Control Plane node
deploy: build-linux
	scp bin/$(BINARY)-linux $(CP_HOST):/tmp/$(BINARY)
	ssh $(CP_HOST) "sudo cp /tmp/$(BINARY) /usr/local/bin/$(BINARY) && sudo chmod +x /usr/local/bin/$(BINARY)"
	scp configs/agctl.yaml $(CP_HOST):/tmp/agctl.yaml
	ssh $(CP_HOST) "sudo mkdir -p /etc/agctl && sudo cp /tmp/agctl.yaml /etc/agctl/config.yaml"
	@echo "✅ $(BINARY) v$(VERSION) deployed"

## Run tests
test:
	go test ./...

## Clean build artifacts
clean:
	rm -rf bin/

## Show help
help:
	@echo "agctl Makefile"
	@echo ""
	@echo "  make build        Build for current OS"
	@echo "  make build-linux  Cross-compile for Linux (Control Plane)"
	@echo "  make install      Install locally"
	@echo "  make deploy       Build + deploy to Control Plane"
	@echo "  make test         Run tests"
	@echo "  make clean        Clean build artifacts"
