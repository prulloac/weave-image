#!/usr/bin/env bash
set -eu

REPO="prulloac/weave-image"
BINARY="weave-image"

usage() {
  cat <<'EOF'
Install weave-image from GitHub Releases without compiling.

Usage: install.sh [options]

Options:
  --version <tag>    Install a specific release tag (default: latest)
  --bin-dir <dir>    Install directory (default: $XDG_BIN_HOME or ~/.local/bin)
  --no-modify-path   Do not edit your shell profile
  --no-ai            Do not install or upgrade weave-ai
  -h, --help         Show this help

Environment:
  WEAVE_IMAGE_VERSION          Same as --version
  WEAVE_IMAGE_BIN_DIR          Same as --bin-dir
  WEAVE_IMAGE_NO_MODIFY_PATH   Set to 1 to skip shell profile changes
  WEAVE_IMAGE_NO_AI            Set to 1 to skip weave-ai installation
  GITHUB_TOKEN              Optional token for the GitHub API rate limit

The installer is user-level by default and never uses sudo.
EOF
}

error() {
  printf 'install: %s\n' "$*" >&2
  exit 1
}

version="${WEAVE_IMAGE_VERSION:-latest}"
bin_dir="${WEAVE_IMAGE_BIN_DIR:-}"
modify_path=1
if [ "${WEAVE_IMAGE_NO_MODIFY_PATH:-}" = "1" ]; then
  modify_path=0
fi
no_ai=0
if [ "${WEAVE_IMAGE_NO_AI:-}" = "1" ]; then
  no_ai=1
fi

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || error "--version requires a tag"
      version="$2"
      shift 2
      ;;
    --version=*)
      version="${1#*=}"
      shift
      ;;
    --bin-dir)
      [ $# -ge 2 ] || error "--bin-dir requires a directory"
      bin_dir="$2"
      shift 2
      ;;
    --bin-dir=*)
      bin_dir="${1#*=}"
      shift
      ;;
    --no-modify-path)
      modify_path=0
      shift
      ;;
    --no-ai)
      no_ai=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      error "unknown option: $1"
      ;;
  esac
done

if [ -z "$bin_dir" ]; then
  bin_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"
fi

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  MINGW*|MSYS*|CYGWIN*)
    error "Windows detected; run scripts/install.ps1 in PowerShell instead"
    ;;
  *)
    error "unsupported operating system: $(uname -s)"
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) error "unsupported architecture: $(uname -m)" ;;
esac

if command -v curl >/dev/null 2>&1; then
  download() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  download() { wget -qO "$2" "$1"; }
else
  error "curl or wget is required"
fi

if [ "$version" = "latest" ]; then
  latest_url="https://api.github.com/repos/${REPO}/releases/latest"
  if command -v curl >/dev/null 2>&1; then
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      response="$(curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" "$latest_url")"
    else
      response="$(curl -fsSL "$latest_url")"
    fi
  else
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      response="$(wget -qO- --header="Authorization: Bearer ${GITHUB_TOKEN}" "$latest_url")"
    else
      response="$(wget -qO- "$latest_url")"
    fi
  fi
  tag="$(printf '%s' "$response" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$tag" ] || error "could not resolve the latest release"
else
  tag="$version"
  case "$tag" in
    v*) ;;
    *) tag="v${tag}" ;;
  esac
fi
version="${tag#v}"

archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
checksums="${BINARY}_${version}_checksums.txt"
base="${WEAVE_IMAGE_RELEASE_BASE:-https://github.com/${REPO}/releases/download/${tag}}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

download "${base}/${archive}" "${tmp}/${archive}"
download "${base}/${checksums}" "${tmp}/${checksums}"

expected="$(awk -v file="$archive" '$2 == file { print $1; exit }' "${tmp}/${checksums}")"
[ -n "$expected" ] || error "checksum not found for ${archive}"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${tmp}/${archive}" | awk '{ print $1 }')"
else
  actual="$(shasum -a 256 "${tmp}/${archive}" | awk '{ print $1 }')"
fi
[ "$expected" = "$actual" ] || error "checksum verification failed for ${archive}"

mkdir -p "${tmp}/extract"
tar -xzf "${tmp}/${archive}" -C "${tmp}/extract"
[ -f "${tmp}/extract/${BINARY}" ] || error "archive does not contain ${BINARY}"

mkdir -p "$bin_dir"
[ -w "$bin_dir" ] || error "install directory is not writable: ${bin_dir} (use --bin-dir)"
cp "${tmp}/extract/${BINARY}" "${bin_dir}/${BINARY}"
chmod 0755 "${bin_dir}/${BINARY}"

profile_for_shell() {
  shell_name="$(basename "${SHELL:-sh}")"
  case "$shell_name" in
    zsh)
      printf '%s\n' "${HOME}/.zshrc"
      ;;
    fish)
      printf '%s\n' "${HOME}/.config/fish/config.fish"
      ;;
    bash)
      if [ -f "${HOME}/.bash_profile" ]; then
        printf '%s\n' "${HOME}/.bash_profile"
      elif [ -f "${HOME}/.bashrc" ]; then
        printf '%s\n' "${HOME}/.bashrc"
      else
        printf '%s\n' "${HOME}/.profile"
      fi
      ;;
    *)
      printf '%s\n' "${HOME}/.profile"
      ;;
  esac
}

case ":${PATH:-}:" in
  *":${bin_dir}:"*)
    echo "${bin_dir} is already on PATH"
    ;;
  *)
    if [ "$modify_path" -eq 1 ]; then
      profile="$(profile_for_shell)"
      mkdir -p "$(dirname "$profile")"
      marker="# Added by the ${BINARY} installer"
      if ! grep -qF "$marker" "$profile" 2>/dev/null; then
        case "$profile" in
          */config.fish)
            printf '\n%s\nfish_add_path "%s"\n' "$marker" "$bin_dir" >>"$profile"
            ;;
          *)
            printf '\n%s\nexport PATH="%s:$PATH"\n' "$marker" "$bin_dir" >>"$profile"
            ;;
        esac
      fi
      echo "Added ${bin_dir} to PATH in ${profile}"
      echo "Restart your shell or run: source ${profile}"
    else
      echo "Add ${bin_dir} to PATH manually:"
      case "${SHELL:-}" in
        */fish) echo "  fish_add_path \"${bin_dir}\"" ;;
        *) echo "  export PATH=\"${bin_dir}:\$PATH\"" ;;
      esac
    fi
    ;;
esac

version_at_least() {
  awk -v have="$1" -v need="$2" 'BEGIN {
    sub(/^v/, "", have); split(have, a, "."); split(need, b, ".");
    for (i = 1; i <= 3; i++) {
      if ((a[i] + 0) > (b[i] + 0)) exit 0
      if ((a[i] + 0) < (b[i] + 0)) exit 1
    }
    exit 0
  }'
}

ensure_weave_ai() {
  if [ "$no_ai" -eq 1 ]; then
    echo "Skipping weave-ai installation (--no-ai)"
    return
  fi
  ai_path=""
  if [ -x "${bin_dir}/weave-ai" ]; then
    ai_path="${bin_dir}/weave-ai"
  elif command -v weave-ai >/dev/null 2>&1; then
    ai_path="$(command -v weave-ai)"
  fi
  if [ -n "$ai_path" ]; then
    ai_version="$("$ai_path" --version 2>/dev/null | awk '{ print $2; exit }')"
    if [ -n "$ai_version" ] && version_at_least "$ai_version" "1.0.0"; then
      echo "weave-ai ${ai_version} is already installed"
      return
    fi
    echo "weave-ai is missing vision support; installing >= 1.0.0"
  else
    echo "Installing weave-ai >= 1.0.0"
  fi
  ai_install_args=(--bin-dir "$bin_dir")
  if [ "$modify_path" -eq 0 ]; then
    ai_install_args+=(--no-modify-path)
  fi
  curl -fsSL https://raw.githubusercontent.com/prulloac/weave-ai/main/scripts/install.sh \
    | bash -s -- "${ai_install_args[@]}"
  ai_path="${bin_dir}/weave-ai"
  [ -x "$ai_path" ] || error "weave-ai installation failed"
  ai_version="$("$ai_path" --version 2>/dev/null | awk '{ print $2; exit }')"
  version_at_least "$ai_version" "1.0.0" ||
    error "weave-ai ${ai_version} is older than 1.0.0"
  echo "Installed weave-ai ${ai_version}"
}

ensure_weave_ai

echo "Installed ${BINARY} ${version} to ${bin_dir}/${BINARY}"
"${bin_dir}/${BINARY}" --version
