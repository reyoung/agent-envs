BINARY_NAME ?= proxy
BIN_DIR ?= bin
OUTPUT := $(BIN_DIR)/$(BINARY_NAME)

GOOS ?= linux
GOARCH ?= amd64

.PHONY: linux clean

linux: $(OUTPUT)

$(OUTPUT):
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o $(OUTPUT) .

clean:
	rm -rf $(BIN_DIR)
