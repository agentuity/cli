
set -e

if [[ -t 1 ]]; then
  tty_escape() { printf "\033[%sm" "$1"; }
else
  tty_escape() { :; }
fi
tty_mkbold() { tty_escape "1;$1"; }
tty_blue="$(tty_mkbold 34)"
tty_red="$(tty_mkbold 31)"
tty_bold="$(tty_mkbold 39)"
tty_reset="$(tty_escape 0)"

ohai() {
  printf "${tty_blue}==>${tty_bold} %s${tty_reset}\n" "$1"
}

warn() {
  printf "${tty_red}Warning${tty_reset}: %s\n" "$1" >&2
}

abort() {
  printf "${tty_red}Error${tty_reset}: %s\n" "$1" >&2
  exit 1
}

OS="$(uname -s)"
ARCH="$(uname -m)"

if [[ "$ARCH" == "x86_64" ]]; then
  ARCH="x86_64"
elif [[ "$ARCH" == "amd64" ]]; then
  ARCH="x86_64"
elif [[ "$ARCH" == "arm64" ]]; then
  ARCH="arm64"
elif [[ "$ARCH" == "aarch64" ]]; then
  ARCH="arm64"
else
  abort "Unsupported architecture: $ARCH"
fi

if [[ "$OS" == "Darwin" ]]; then
  OS="Darwin"
  EXTENSION="tar.gz"
  INSTALL_DIR="/usr/local/bin"
elif [[ "$OS" == "Linux" ]]; then
  OS="Linux"
  EXTENSION="tar.gz"
  INSTALL_DIR="/usr/local/bin"
elif [[ "$OS" == "MINGW"* ]] || [[ "$OS" == "MSYS"* ]] || [[ "$OS" == "CYGWIN"* ]]; then
  OS="Windows"
  EXTENSION="zip"
  INSTALL_DIR="$HOME/bin"
else
  abort "Unsupported operating system: $OS"
fi

VERSION="latest"
INSTALL_PATH="$INSTALL_DIR"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version)
      VERSION="$2"
      shift 2
      ;;
    -d|--dir)
      INSTALL_PATH="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: install.sh [options]"
      echo "Options:"
      echo "  -v, --version VERSION    Install specific version"
      echo "  -d, --dir DIRECTORY      Install to specific directory"
      echo "  -h, --help               Show this help message"
      exit 0
      ;;
    *)
      warn "Unknown option: $1"
      shift
      ;;
  esac
done

if [[ ! -d "$INSTALL_PATH" ]]; then
  ohai "Creating install directory: $INSTALL_PATH"
  mkdir -p "$INSTALL_PATH" || abort "Failed to create directory: $INSTALL_PATH"
fi

if [[ ! -w "$INSTALL_PATH" ]]; then
  abort "No write permission to $INSTALL_PATH. Try running with sudo or specify a different directory with --dir."
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

if [[ "$VERSION" == "latest" ]]; then
  ohai "Fetching latest release information..."
  RELEASE_URL="https://api.github.com/repos/agentuity/cli/releases/latest"
  VERSION=$(curl -s $RELEASE_URL | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
  if [[ -z "$VERSION" ]]; then
    abort "Failed to fetch latest version information"
  fi
fi

VERSION="${VERSION#v}"
DOWNLOAD_FILENAME="agentuity_${OS}_${ARCH}.${EXTENSION}"
DOWNLOAD_URL="https://github.com/agentuity/cli/releases/download/v${VERSION}/${DOWNLOAD_FILENAME}"

ohai "Downloading Agentuity CLI v${VERSION} for ${OS}/${ARCH}..."
curl -L --progress-bar "$DOWNLOAD_URL" -o "$TMP_DIR/$DOWNLOAD_FILENAME" || abort "Failed to download from $DOWNLOAD_URL"

ohai "Extracting..."
if [[ "$EXTENSION" == "tar.gz" ]]; then
  tar -xzf "$TMP_DIR/$DOWNLOAD_FILENAME" -C "$TMP_DIR" || abort "Failed to extract archive"
elif [[ "$EXTENSION" == "zip" ]]; then
  unzip -q "$TMP_DIR/$DOWNLOAD_FILENAME" -d "$TMP_DIR" || abort "Failed to extract archive"
else
  abort "Unknown archive format: $EXTENSION"
fi

ohai "Installing to $INSTALL_PATH..."
if [[ -f "$TMP_DIR/agentuity" ]]; then
  cp "$TMP_DIR/agentuity" "$INSTALL_PATH/" || abort "Failed to copy binary to $INSTALL_PATH"
  chmod +x "$INSTALL_PATH/agentuity" || abort "Failed to make binary executable"
else
  abort "Binary not found in extracted archive"
fi

if command -v "$INSTALL_PATH/agentuity" >/dev/null 2>&1; then
  ohai "Successfully installed Agentuity CLI to $INSTALL_PATH/agentuity"
else
  abort "Installation verification failed"
fi

if [[ ":$PATH:" != *":$INSTALL_PATH:"* ]]; then
  warn "$INSTALL_PATH is not in your PATH. You may need to add it to use agentuity command."
  case "$SHELL" in
    */bash*)
      echo "  echo 'export PATH=\"\$PATH:$INSTALL_PATH\"' >> ~/.bashrc"
      ;;
    */zsh*)
      echo "  echo 'export PATH=\"\$PATH:$INSTALL_PATH\"' >> ~/.zshrc"
      ;;
    *)
      echo "  Add $INSTALL_PATH to your PATH"
      ;;
  esac
fi

ohai "Installation complete! Run 'agentuity --help' to get started."
echo "For more information, visit: https://agentuity.com"
