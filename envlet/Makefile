BINARY_NAME ?= envlet
BIN_DIR ?= bin
ENVLET_OUTPUT := $(BIN_DIR)/$(BINARY_NAME)
JUDGELET_OUTPUT := $(BIN_DIR)/judgelet

GOOS ?= linux
GOARCH ?= amd64

.PHONY: linux envlet judgelet clean

linux: envlet judgelet

envlet: $(ENVLET_OUTPUT)

judgelet: $(JUDGELET_OUTPUT)

$(ENVLET_OUTPUT):
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o $(ENVLET_OUTPUT) ./cmd/envlet

$(JUDGELET_OUTPUT):
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o $(JUDGELET_OUTPUT) ./cmd/judgelet

clean:
	rm -rf $(BIN_DIR)
