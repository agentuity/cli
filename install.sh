
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
  
  if command -v brew >/dev/null 2>&1 && [[ "$NO_BREW" != "true" ]]; then
      ohai "Homebrew detected! Installing Agentuity CLI using Homebrew..."
      
      if [[ "$VERSION" != "latest" ]]; then
        ohai "Installing Agentuity CLI version $VERSION using Homebrew..."
        FORMULA="agentuity/tap/agentuity@${VERSION}"
        brew install $FORMULA
      else
        ohai "Installing latest Agentuity CLI version using Homebrew..."
        brew install agentuity/tap/agentuity
      fi
      
      if command -v agentuity >/dev/null 2>&1; then
        ohai "Agentuity CLI installed successfully via Homebrew!"
        ohai "Version: $(agentuity --version)"
        exit 0
      else
        abort "Homebrew installation failed. Please try again or use manual installation."
      fi
  fi
elif [[ "$OS" == "Linux" ]]; then
  OS="Linux"
  EXTENSION="tar.gz"
  INSTALL_DIR="/usr/local/bin"
elif [[ "$OS" == "MINGW"* ]] || [[ "$OS" == "MSYS"* ]] || [[ "$OS" == "CYGWIN"* ]]; then
  OS="Windows"
  EXTENSION="msi"
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
    --no-brew)
      NO_BREW=true
      shift
      ;;
    -h|--help)
      echo "Usage: install.sh [options]"
      echo "Options:"
      echo "  -v, --version VERSION    Install specific version"
      echo "  -d, --dir DIRECTORY      Install to specific directory"
      if [[ "$OS" == "Darwin" ]]; then
        echo "  --no-brew               Skip Homebrew installation on macOS"
      fi
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

if [[ "$OS" == "Windows" ]]; then
  if [[ "$ARCH" == "x86_64" ]]; then
    DOWNLOAD_FILENAME="agentuity-x64.msi"
  elif [[ "$ARCH" == "arm64" ]]; then
    DOWNLOAD_FILENAME="agentuity-arm64.msi"
  else
    DOWNLOAD_FILENAME="agentuity-x86.msi"
  fi
else
  DOWNLOAD_FILENAME="agentuity_${OS}_${ARCH}.${EXTENSION}"
fi

DOWNLOAD_URL="https://github.com/agentuity/cli/releases/download/v${VERSION}/${DOWNLOAD_FILENAME}"
CHECKSUMS_URL="https://github.com/agentuity/cli/releases/download/v${VERSION}/checksums.txt"

ohai "Downloading Agentuity CLI v${VERSION} for ${OS}/${ARCH}..."
curl -L --progress-bar "$DOWNLOAD_URL" -o "$TMP_DIR/$DOWNLOAD_FILENAME" || abort "Failed to download from $DOWNLOAD_URL"

if [[ "$OS" != "Windows" ]]; then
  ohai "Downloading checksums for verification..."
  if ! curl -L --silent "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"; then
    warn "Failed to download checksums file. Skipping verification."
  else
    ohai "Verifying checksum..."
    if command -v sha256sum >/dev/null 2>&1; then
      CHECKSUM_TOOL="sha256sum"
    elif command -v shasum >/dev/null 2>&1; then
      CHECKSUM_TOOL="shasum -a 256"
    else
      warn "Neither sha256sum nor shasum found. Skipping checksum verification."
      CHECKSUM_TOOL=""
    fi
    
    if [[ -n "$CHECKSUM_TOOL" ]]; then
      cd "$TMP_DIR"
      COMPUTED_CHECKSUM=$($CHECKSUM_TOOL "$DOWNLOAD_FILENAME" | cut -d ' ' -f 1)
      EXPECTED_CHECKSUM=$(grep "$DOWNLOAD_FILENAME" checksums.txt | cut -d ' ' -f 1)
      
      if [[ -z "$EXPECTED_CHECKSUM" ]]; then
        warn "Checksum for $DOWNLOAD_FILENAME not found in checksums.txt. Skipping verification."
      elif [[ "$COMPUTED_CHECKSUM" != "$EXPECTED_CHECKSUM" ]]; then
        abort "Checksum verification failed. Expected: $EXPECTED_CHECKSUM, Got: $COMPUTED_CHECKSUM"
      else
        ohai "Checksum verification passed!"
      fi
      cd - > /dev/null
    fi
  fi
fi

ohai "Processing download..."
if [[ "$OS" == "Windows" ]]; then
  ohai "Downloaded Windows MSI installer to $TMP_DIR/$DOWNLOAD_FILENAME"
  
  if [[ -n "${CI}" || -n "${GITHUB_ACTIONS}" || -n "${NONINTERACTIVE}" ]]; then
    warn "Non-interactive environment detected, skipping automatic MSI installation"
    cp "$TMP_DIR/$DOWNLOAD_FILENAME" "$HOME/" || abort "Failed to copy MSI to $HOME/"
    ohai "MSI installer copied to $HOME/$DOWNLOAD_FILENAME"
    ohai "To install manually, run the MSI installer at:"
    echo "  $HOME/$DOWNLOAD_FILENAME"
    exit 0
  fi
  
  ohai "Attempting to run MSI installer automatically..."
  if command -v msiexec >/dev/null 2>&1; then
    ohai "Running installer with msiexec..."
    msiexec /i "$TMP_DIR/$DOWNLOAD_FILENAME" /qn /norestart
    INSTALL_STATUS=$?
    if [[ $INSTALL_STATUS -eq 0 ]]; then
      ohai "Installation completed successfully!"
      exit 0
    else
      warn "Automatic installation failed with status: $INSTALL_STATUS"
    fi
  else
    warn "msiexec not found, cannot run installer automatically"
  fi
  
  cp "$TMP_DIR/$DOWNLOAD_FILENAME" "$HOME/" || abort "Failed to copy MSI to $HOME/"
  ohai "MSI installer copied to $HOME/$DOWNLOAD_FILENAME"
  ohai "To install manually, run the MSI installer at:"
  echo "  $HOME/$DOWNLOAD_FILENAME"
  exit 0
elif [[ "$EXTENSION" == "tar.gz" ]]; then
  ohai "Extracting..."
  tar -xzf "$TMP_DIR/$DOWNLOAD_FILENAME" -C "$TMP_DIR" || abort "Failed to extract archive"
elif [[ "$EXTENSION" == "zip" ]]; then
  ohai "Extracting..."
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
  ohai "Adding $INSTALL_PATH to your PATH..."
  
  SHELL_CONFIG=""
  case "$SHELL" in
    */bash*)
      SHELL_CONFIG="$HOME/.bashrc"
      if [[ -f "$HOME/.bash_profile" ]]; then
        SHELL_CONFIG="$HOME/.bash_profile"
      fi
      ;;
    */zsh*)
      SHELL_CONFIG="$HOME/.zshrc"
      ;;
    */fish*)
      SHELL_CONFIG="$HOME/.config/fish/config.fish"
      ;;
  esac
  
  if [[ -n "$SHELL_CONFIG" ]] && [[ -w "$SHELL_CONFIG" ]]; then
    echo "export PATH=\"\$PATH:$INSTALL_PATH\"" >> "$SHELL_CONFIG"
    ohai "Added $INSTALL_PATH to PATH in $SHELL_CONFIG"
    export PATH="$PATH:$INSTALL_PATH"
  else
    warn "$INSTALL_PATH is not in your PATH. You may need to add it manually to use the agentuity command."
    case "$SHELL" in
      */bash*)
        echo "  echo 'export PATH=\"\$PATH:$INSTALL_PATH\"' >> ~/.bashrc"
        ;;
      */zsh*)
        echo "  echo 'export PATH=\"\$PATH:$INSTALL_PATH\"' >> ~/.zshrc"
        ;;
      */fish*)
        echo "  echo 'set -gx PATH \$PATH $INSTALL_PATH' >> ~/.config/fish/config.fish"
        ;;
      *)
        echo "  Add $INSTALL_PATH to your PATH"
        ;;
    esac
  fi
fi

ohai "Setting up shell completions..."
if command -v "$INSTALL_PATH/agentuity" >/dev/null 2>&1; then
  COMPLETION_DIR=""
  
  case "$OS" in
    "Darwin")
      if [[ -d "/usr/local/etc/bash_completion.d" ]]; then
        BASH_COMPLETION_DIR="/usr/local/etc/bash_completion.d"
        if [[ -w "$BASH_COMPLETION_DIR" ]]; then
          ohai "Generating bash completion script..."
          "$INSTALL_PATH/agentuity" completion bash > "$BASH_COMPLETION_DIR/agentuity"
          ohai "Bash completion installed to $BASH_COMPLETION_DIR/agentuity"
        else
          warn "No write permission to $BASH_COMPLETION_DIR. Skipping bash completion installation."
        fi
      fi
      
      if [[ -d "/usr/local/share/zsh/site-functions" ]]; then
        ZSH_COMPLETION_DIR="/usr/local/share/zsh/site-functions"
        if [[ -w "$ZSH_COMPLETION_DIR" ]]; then
          ohai "Generating zsh completion script..."
          "$INSTALL_PATH/agentuity" completion zsh > "$ZSH_COMPLETION_DIR/_agentuity"
          ohai "Zsh completion installed to $ZSH_COMPLETION_DIR/_agentuity"
        else
          warn "No write permission to $ZSH_COMPLETION_DIR. Skipping zsh completion installation."
        fi
      fi
      ;;
    "Linux")
      if [[ -d "/etc/bash_completion.d" ]]; then
        BASH_COMPLETION_DIR="/etc/bash_completion.d"
        if [[ -w "$BASH_COMPLETION_DIR" ]]; then
          ohai "Generating bash completion script..."
          "$INSTALL_PATH/agentuity" completion bash > "$BASH_COMPLETION_DIR/agentuity"
          ohai "Bash completion installed to $BASH_COMPLETION_DIR/agentuity"
        else
          warn "No write permission to $BASH_COMPLETION_DIR. Skipping bash completion installation."
          ohai "You can manually install bash completion with:"
          echo "  $INSTALL_PATH/agentuity completion bash > ~/.bash_completion"
        fi
      fi
      
      if [[ -d "/usr/share/zsh/vendor-completions" ]]; then
        ZSH_COMPLETION_DIR="/usr/share/zsh/vendor-completions"
        if [[ -w "$ZSH_COMPLETION_DIR" ]]; then
          ohai "Generating zsh completion script..."
          "$INSTALL_PATH/agentuity" completion zsh > "$ZSH_COMPLETION_DIR/_agentuity"
          ohai "Zsh completion installed to $ZSH_COMPLETION_DIR/_agentuity"
        else
          warn "No write permission to $ZSH_COMPLETION_DIR. Skipping zsh completion installation."
          ohai "You can manually install zsh completion with:"
          echo "  mkdir -p ~/.zsh/completion"
          echo "  $INSTALL_PATH/agentuity completion zsh > ~/.zsh/completion/_agentuity"
          echo "  echo 'fpath=(~/.zsh/completion \$fpath)' >> ~/.zshrc"
          echo "  echo 'autoload -U compinit && compinit' >> ~/.zshrc"
        fi
      fi
      ;;
    "Windows")
      ohai "You can manually install PowerShell completion with:"
      echo "  $INSTALL_PATH/agentuity completion powershell > agentuity.ps1"
      echo "  Move agentuity.ps1 to a directory in your PowerShell module path"
      ;;
  esac
  
  ohai "To manually set up shell completions, run:"
  echo "  For bash: $INSTALL_PATH/agentuity completion bash > /path/to/bash_completion.d/agentuity"
  echo "  For zsh:  $INSTALL_PATH/agentuity completion zsh > /path/to/zsh/site-functions/_agentuity"
  echo "  For fish: $INSTALL_PATH/agentuity completion fish > ~/.config/fish/completions/agentuity.fish"
fi

ohai "Installation complete! Run 'agentuity --help' to get started."
echo "For more information, visit: https://agentuity.com"
