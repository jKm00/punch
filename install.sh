#!/usr/bin/env bash
#
# punch installer.
#
# Usage (one-liner):
#   curl -fsSL https://raw.githubusercontent.com/jKm00/punch/main/install.sh | bash
#
# The script:
#   1. detects the OS/arch,
#   2. finds the latest release via the GitHub API,
#   3. downloads the matching tarball + SHA256SUMS,
#   4. verifies the checksum,
#   5. installs the `punch` binary to ~/.local/bin,
#   6. warns if ~/.local/bin is not on your PATH.
#
# The repo is public, so no token is needed. (If you ever make it private, set
# GITHUB_TOKEN before running.)

set -euo pipefail

# --- Configuration (single source of truth) ---------------------------------
OWNER="jKm00"
REPO="punch"
API_BASE="https://api.github.com"
INSTALL_DIR="${HOME}/.local/bin"
BINARY="punch"

# --- Helpers -----------------------------------------------------------------
info() { printf '%s\n' "$*" >&2; }
err()  { printf 'error: %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

cleanup() {
  [ -n "${TMPDIR_INSTALL:-}" ] && rm -rf "${TMPDIR_INSTALL}" 2>/dev/null || true
}
trap cleanup EXIT

need() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# Token from the environment, if any (optional for public repos).
token() {
  if [ -n "${GITHUB_TOKEN:-}" ];       then printf '%s' "${GITHUB_TOKEN}";       return; fi
  if [ -n "${PUNCH_GITHUB_TOKEN:-}" ]; then printf '%s' "${PUNCH_GITHUB_TOKEN}"; return; fi
  printf ''
}

# curl wrapper that adds auth + API version headers and fails on HTTP errors.
api_curl() {
  local accept="$1"; shift
  local tok; tok="$(token)"
  local args=(-fsSL -H "Accept: ${accept}" -H "X-GitHub-Api-Version: 2022-11-28")
  if [ -n "${tok}" ]; then
    args+=(-H "Authorization: Bearer ${tok}")
  fi
  curl "${args[@]}" "$@"
}

# --- Detect platform ---------------------------------------------------------
detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux)  printf 'linux' ;;
    *) die "unsupported OS: $(uname -s) (only macOS and Linux are supported)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    arm64|aarch64) printf 'arm64' ;;
    x86_64|amd64)  printf 'amd64' ;;
    *) die "unsupported architecture: $(uname -m) (only amd64 and arm64 are supported)" ;;
  esac
}

# --- Fetch release metadata --------------------------------------------------
# Echo the JSON of the latest release; give a clear message on auth failure.
fetch_release_json() {
  local url="${API_BASE}/repos/${OWNER}/${REPO}/releases/latest"
  local out status
  # Capture HTTP status separately so we can explain auth errors.
  out="$(api_curl "application/vnd.github+json" -w $'\n%{http_code}' "${url}" 2>/dev/null)" || {
    status="${out##*$'\n'}"
    if [ "${status}" = "404" ]; then
      die "no releases found at ${url} (has a version been released yet?)."
    fi
    if [ "${status}" = "401" ] || [ "${status}" = "403" ]; then
      die "GitHub returned ${status} reading releases. If this repo is private,
set a token first:  export GITHUB_TOKEN=<your-token>  then re-run."
    fi
    die "could not reach the releases API at ${url} (curl failed)."
  }
  # Strip the trailing status line.
  printf '%s' "${out%$'\n'*}"
}

# Given the release JSON and an asset name, echo the asset's API URL.
asset_api_url() {
  local json="$1" name="$2"
  if command -v jq >/dev/null 2>&1; then
    printf '%s' "${json}" | jq -r --arg n "${name}" '.assets[] | select(.name == $n) | .url'
    return
  fi
  # Portable fallback: assets are objects with "url" and "name" fields. Find the
  # object whose "name" matches and print its "url". We normalize to one
  # object per line, then grep. Tolerates zero or more spaces after colons.
  printf '%s' "${json}" \
    | tr '{' '\n' \
    | grep -E "\"name\": *\"${name}\"" \
    | grep -oE "\"url\": *\"[^\"]*\"" \
    | head -n1 \
    | sed -E 's/.*"url": *"([^"]*)".*/\1/'
}

tag_name() {
  local json="$1"
  if command -v jq >/dev/null 2>&1; then
    printf '%s' "${json}" | jq -r '.tag_name'
    return
  fi
  printf '%s' "${json}" | grep -oE "\"tag_name\": *\"[^\"]*\"" | head -n1 | sed -E 's/.*"tag_name": *"([^"]*)".*/\1/'
}

# --- Main --------------------------------------------------------------------
main() {
  need curl
  need uname
  need tar

  local os arch
  os="$(detect_os)"
  arch="$(detect_arch)"

  info "Looking up the latest punch release…"
  local json tag ver
  json="$(fetch_release_json)"
  tag="$(tag_name "${json}")"
  [ -n "${tag}" ] || die "could not determine the latest release tag."
  ver="${tag#v}"

  local asset="${REPO}_${ver}_${os}_${arch}.tar.gz"
  local asset_url sums_url
  asset_url="$(asset_api_url "${json}" "${asset}")"
  sums_url="$(asset_api_url "${json}" "SHA256SUMS")"
  [ -n "${asset_url}" ] || die "release ${tag} has no asset for your platform (${asset})."
  [ -n "${sums_url}" ] || die "release ${tag} is missing its SHA256SUMS asset."

  TMPDIR_INSTALL="$(mktemp -d)"
  local tarball="${TMPDIR_INSTALL}/${asset}"
  local sumsfile="${TMPDIR_INSTALL}/SHA256SUMS"

  info "Downloading punch ${tag} (${os}/${arch})…"
  api_curl "application/octet-stream" -o "${tarball}" "${asset_url}"
  api_curl "application/octet-stream" -o "${sumsfile}" "${sums_url}"

  info "Verifying checksum…"
  local want got
  want="$(grep -E "[ *.]${asset}\$" "${sumsfile}" | awk '{print $1}' | head -n1)"
  [ -n "${want}" ] || die "no checksum for ${asset} in SHA256SUMS."
  if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "${tarball}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    got="$(shasum -a 256 "${tarball}" | awk '{print $1}')"
  else
    die "need sha256sum or shasum to verify the download."
  fi
  [ "${want}" = "${got}" ] || die "checksum mismatch for ${asset} (expected ${want}, got ${got})."

  info "Installing to ${INSTALL_DIR}/${BINARY}…"
  tar -xzf "${tarball}" -C "${TMPDIR_INSTALL}"
  [ -f "${TMPDIR_INSTALL}/${BINARY}" ] || die "archive did not contain the ${BINARY} binary."
  mkdir -p "${INSTALL_DIR}"
  install -m 0755 "${TMPDIR_INSTALL}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

  info ""
  info "Installed punch ${tag} to ${INSTALL_DIR}/${BINARY}"

  print_getting_started() {
    info ""
    info "Using the default config — run 'punch setup --curr' to see it, or 'punch setup' to change it."
    info "Run 'punch help' for usage."
  }

  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*)
      print_getting_started
      ;;
    *)
      info ""
      info "NOTE: ${INSTALL_DIR} is not on your PATH. Add this to your shell profile:"
      info "    export PATH=\"${INSTALL_DIR}:\$PATH\""
      info "Then restart your shell."
      print_getting_started
      ;;
  esac
}

main "$@"
