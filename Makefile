BINARY := wh
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build install test vet tidy clean

build:
	go build -o ./$(BINARY) ./cmd/wh

install:
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/wh

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f ./$(BINARY)
