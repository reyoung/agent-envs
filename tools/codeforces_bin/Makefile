JSONL ?= ../../datasets/codeforces_problems.jsonl
DATA ?= cmd/cf_case/data/codeforces_problems.bin
BIN ?= bin/cf_case

.PHONY: all build generate clean linux

all: build

generate:
	@mkdir -p cmd/cf_case/data
	go run ./cmd/cf_bin_builder -input $(JSONL) -output $(DATA)

build: generate
	@mkdir -p bin
	CGO_ENABLED=0 go build -o $(BIN) ./cmd/cf_case

linux: generate
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN) ./cmd/cf_case

clean:
	rm -f $(DATA) $(BIN)
