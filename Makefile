# punch - work-hour tracker

BINARY := punch
INSTALL_DIR := $(HOME)/.local/bin

# Version stamped into the binary via -ldflags. Local `build`/`install` leave it
# as "dev" (so they are never nagged to upgrade and cannot self-replace); only
# release builds pass an explicit VERSION (see `release-build` and CI).
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

# --- Dev environment ---------------------------------------------------------
# Dev commands load variables from a local, gitignored .env file (if present),
# then run the tool from source with `go run`. The .env is ONLY sourced by these
# make targets -- it is never read by the installed binary, so your real
# database at ~/.local/share/punch/punch.db is never affected by dev settings.
#
# Default dev database lives inside the repo (gitignored) so dev work never
# touches production data. Override by setting PUNCH_DB in .env.
ENV_FILE := .env
DEV_DB ?= ./dev.db

# If a .env exists, export everything it defines to recipe commands.
ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

# PUNCH_DB precedence for dev targets: .env value (if any) wins, else DEV_DB.
PUNCH_DB ?= $(DEV_DB)

.PHONY: build install uninstall release-build test vet tidy clean dev dev-build dev-db-path dev-reset env

# --- Production --------------------------------------------------------------

build:
	go build -ldflags "$(LDFLAGS)" -o ./$(BINARY) ./cmd/punch

install:
	go build -ldflags "$(LDFLAGS)" -o $(INSTALL_DIR)/$(BINARY) ./cmd/punch

# release-build cross-compiles a stamped binary for GOOS/GOARCH into OUT.
# Used by CI; requires VERSION, GOOS, GOARCH, OUT to be set.
#   make release-build VERSION=v1.0.0 GOOS=linux GOARCH=amd64 OUT=dist/punch
release-build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUT) ./cmd/punch

# Remove the installed binary. Your data (~/.local/share/punch/punch.db) is kept.
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "Removed $(INSTALL_DIR)/$(BINARY)."
	@echo "Your data is kept at ~/.local/share/punch/punch.db (delete it manually to remove)."

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f ./$(BINARY)

# --- Development -------------------------------------------------------------

# Run the tool from source against the dev database.
# Pass a subcommand and args via ARGS, e.g.:
#   make dev ARGS="in"
#   make dev ARGS="week last"
#   make dev ARGS='set 15.02 --start 08:00 --end 16:00'
dev:
	PUNCH_DB="$(PUNCH_DB)" go run ./cmd/punch $(ARGS)

# Build a dev binary into the repo (gitignored) for repeated manual runs.
# Run it with: PUNCH_DB=./dev.db ./punch <cmd>
dev-build: build

# Print the dev database path that `make dev` will use.
dev-db-path:
	@echo "$(PUNCH_DB)"

# Delete the dev database (and its WAL/SHM sidecars).
dev-reset:
	rm -f "$(PUNCH_DB)" "$(PUNCH_DB)-wal" "$(PUNCH_DB)-shm"

# Show the effective dev environment.
env:
	@echo "ENV_FILE  = $(ENV_FILE)"
	@echo "PUNCH_DB  = $(PUNCH_DB)"
