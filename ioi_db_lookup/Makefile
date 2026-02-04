.PHONY: build clean

BINARY := bin/ioi_db_lookup

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BINARY) ./

clean:
	rm -rf bin
