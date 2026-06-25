# wh - work-hour tracker

BINARY := wh
INSTALL_DIR := $(HOME)/.local/bin

# --- Dev environment ---------------------------------------------------------
# Dev commands load variables from a local, gitignored .env file (if present),
# then run the tool from source with `go run`. The .env is ONLY sourced by these
# make targets -- it is never read by the installed binary, so your real
# database at ~/.local/share/wh/wh.db is never affected by dev settings.
#
# Default dev database lives inside the repo (gitignored) so dev work never
# touches production data. Override by setting WH_DB in .env.
ENV_FILE := .env
DEV_DB ?= ./dev.db

# If a .env exists, export everything it defines to recipe commands.
ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

# WH_DB precedence for dev targets: .env value (if any) wins, else DEV_DB.
WH_DB ?= $(DEV_DB)

.PHONY: build install uninstall test vet tidy clean dev dev-build dev-db-path dev-reset env

# --- Production --------------------------------------------------------------

build:
	go build -o ./$(BINARY) ./cmd/wh

install:
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/wh

# Remove the installed binary. Your data (~/.local/share/wh/wh.db) is kept.
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "Removed $(INSTALL_DIR)/$(BINARY)."
	@echo "Your data is kept at ~/.local/share/wh/wh.db (delete it manually to remove)."

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
	WH_DB="$(WH_DB)" go run ./cmd/wh $(ARGS)

# Build a dev binary into the repo (gitignored) for repeated manual runs.
# Run it with: WH_DB=./dev.db ./wh <cmd>
dev-build: build

# Print the dev database path that `make dev` will use.
dev-db-path:
	@echo "$(WH_DB)"

# Delete the dev database (and its WAL/SHM sidecars).
dev-reset:
	rm -f "$(WH_DB)" "$(WH_DB)-wal" "$(WH_DB)-shm"

# Show the effective dev environment.
env:
	@echo "ENV_FILE = $(ENV_FILE)"
	@echo "WH_DB    = $(WH_DB)"
