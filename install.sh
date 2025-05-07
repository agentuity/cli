#!/bin/bash

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
tty_cyan="$(tty_mkbold 36)"
tty_magenta="$(tty_mkbold 35)"
tty_underline="$(tty_escape 4)"

USE_BREW="true"
CAT=${CAT:-cat}
ARCH=$(uname -m)
OS=$(uname -s)
EXTENSION="tar.gz"
DEBUG="false"

usage() {
$CAT 1>&2 <<EOF
Usage: install.sh [options]

Options:
  -v, --version VERSION    Install specific version
  -d, --dir DIRECTORY      Install to specific directory
  -h, --help               Show this help message
  -n, --no-brew            Do not use Homebrew to install Agentuity CLI (default: false)
EOF
}

parse_cli_args() {
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
        usage
        exit 0
        ;;
      -n|--no-brew)
        USE_BREW="false"
        shift
        ;;
    esac
  done

  debug "  > VERSION: $VERSION"
  debug "  > INSTALL_PATH: $INSTALL_PATH"
  debug "  > USE_BREW: $USE_BREW"
}

debug() {
  if [[ "$DEBUG" == "true" ]]; then
    printf "${tty_magenta}[DEBUG] ${tty_bold} %s${tty_reset}\n" "$1"
  fi
}

ohai() {
  printf "${tty_blue}==>${tty_bold} %s${tty_reset}\n" "$1"
}

url() {
  printf "${tty_cyan}${tty_underline}%s${tty_reset}" "$1"
}

warn() {
  printf "${tty_red}Warning${tty_reset}: %s\n" "$1" >&2
}

abort() {
  printf "${tty_red}Error${tty_reset}: %s\n" "$1" >&2
  exit 1
}


check_known_arch() {
  if [[ "$ARCH" != "x86_64" ]] && [[ "$ARCH" != "amd64" ]] && [[ "$ARCH" != "arm64" ]] && [[ "$ARCH" != "aarch64" ]]; then
    abort "Unsupported architecture: $ARCH"
  fi

  if [[ "$ARCH" == "x86_64" || "$ARCH" == "amd64" ]]; then
    ARCH="x86_64"
  else
    ARCH="arm64"
  fi

  debug "  > Architecture: $ARCH"
}

is_macos() {
  if [[ "$OS" == "Darwin" ]]; then
    return 0
  else
    return 1
  fi
}

is_linux() {
  if [[ "$OS" == "Linux" ]]; then
    return 0
  else
    return 1
  fi
}

abort_if_windows() {
  if [[ "$OS" == "Windows" ]]; then
    abort "Windows is not supported. Please use WSL or a native Windows installation."
  fi
}

setup_default_install_path_var() {
  if is_macos; then
    if [[ -d "$HOME/.local/bin" ]] && [[ -w "$HOME/.local/bin" ]]; then
      INSTALL_PATH="$HOME/.local/bin"
    elif [[ -d "$HOME/.bin" ]] && [[ -w "$HOME/.bin" ]]; then
      INSTALL_PATH="$HOME/.bin"
    elif [[ -d "$HOME/bin" ]] && [[ -w "$HOME/bin" ]]; then
      INSTALL_PATH="$HOME/bin"
    else
      INSTALL_PATH="/usr/local/bin"
    fi
  elif is_linux; then
    if [[ -d "$HOME/.local/bin" ]] && [[ -w "$HOME/.local/bin" ]]; then
      INSTALL_PATH="$HOME/.local/bin"
    elif [[ -d "$HOME/.bin" ]] && [[ -w "$HOME/.bin" ]]; then
      INSTALL_PATH="$HOME/.bin"
    elif [[ -d "$HOME/bin" ]] && [[ -w "$HOME/bin" ]]; then
      INSTALL_PATH="$HOME/bin"
    else
      INSTALL_PATH="/usr/local/bin"
    fi
  else
    abort "Unsupported operating system: $OS"
  fi

  debug "  > Install Path: $INSTALL_PATH"
}

check_install_path() {

  if [[ ! -d "$INSTALL_PATH" ]]; then
    ohai "Creating install directory: $INSTALL_PATH"
    mkdir -p "$INSTALL_PATH" 2>/dev/null || true  # Don't abort if mkdir fails
  fi

  if [[ -w "$INSTALL_PATH" ]]; then
    ohai "No write permission to $INSTALL_PATH. Trying alternative locations..."
  fi

   
  if [[ "$INSTALL_PATH" == "/usr/local/bin" ]]; then
    ohai "No write permission to $INSTALL_PATH. Trying alternative locations..."
    
    if [[ -d "$HOME/.local/bin" ]] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
      if [[ -w "$HOME/.local/bin" ]]; then
        ohai "Using $HOME/.local/bin instead"
        INSTALL_PATH="$HOME/.local/bin"
      fi
    elif [[ -d "$HOME/.bin" ]] || mkdir -p "$HOME/.bin" 2>/dev/null; then
      if [[ -w "$HOME/.bin" ]]; then
        ohai "Using $HOME/.bin instead"
        INSTALL_PATH="$HOME/.bin"
      fi
    elif [[ -d "$HOME/bin" ]] || mkdir -p "$HOME/bin" 2>/dev/null; then
      if [[ -w "$HOME/bin" ]]; then
        ohai "Using $HOME/bin instead"
        INSTALL_PATH="$HOME/bin"
      fi
    else
      abort "Could not find or create a writable installation directory. Try running with sudo or specify a different directory with --dir."
    fi
    debug "  > Install Path Changed: $INSTALL_PATH"
  fi

  if [[ ! -w "$INSTALL_PATH" ]]; then
    abort "No write permission to $INSTALL_PATH. Try running with sudo or specify a different directory with --dir."
  fi
}

check_version() {
    if [[ -z "$VERSION" ]]; then
      abort "Version is empty. This should not happen."
    fi

    debug "  > Version: $VERSION"
}

check_latest_release() {
  if [[ "$VERSION" == "latest" ]]; then
    ohai "Fetching latest release information..."
    VERSION=$(curl -s $"https://agentuity.sh/release/cli" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [[ -z "$VERSION" ]]; then
      abort "Failed to fetch latest version information"
    fi
  fi

  debug "  > Version: $VERSION"

}

install_mac() {
  if command -v brew >/dev/null 2>&1 && [[ "$USE_BREW" == "true" ]]; then

    ohai "Homebrew detected! Installing Agentuity CLI using Homebrew..."
      
    if [[ "$VERSION" != "latest" ]]; then
      ohai "Installing Agentuity CLI version $VERSION using Homebrew..."
      brew install agentuity/tap/agentuity@${VERSION#v}
    else

    debug "  > heloooooooo 2?: $USE_BREW"
      ohai "Installing latest Agentuity CLI version using Homebrew..."
      brew install -q agentuity/tap/agentuity
    fi

    if command -v agentuity >/dev/null 2>&1; then
      success
    else
      abort "Homebrew installation failed. Please try again or use manual installation."
    fi

  else
      ohai "Installing Agentuity CLI using curl..."
  fi

}


download_release() {

  DOWNLOAD_FILENAME="agentuity_${OS}_${ARCH}.${EXTENSION}"
  DOWNLOAD_URL="https://github.com/agentuity/cli/releases/download/v${VERSION#v}/${DOWNLOAD_FILENAME}"

  debug "  > DOWNLOAD_URL: $DOWNLOAD_URL"
  debug "  > DOWNLOAD_FILENAME: $DOWNLOAD_FILENAME"
  debug "  > TMP_DIR: $TMP_DIR"

  if curl --head --silent --fail "$DOWNLOAD_URL" > /dev/null 2>&1; then
    ohai "Downloading Agentuity CLI v${VERSION} for ${OS}/${ARCH}..."
    curl -L --progress-bar "$DOWNLOAD_URL" -o "$TMP_DIR/$DOWNLOAD_FILENAME" || abort "Failed to download from $DOWNLOAD_URL"
  else
    abort "Failed to download from $DOWNLOAD_URL"
  fi


}

download_checksums() {
  CHECKSUMS_URL="https://github.com/agentuity/cli/releases/download/v${VERSION#v}/checksums.txt"
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
}

extract_release() {
  if [[ "$EXTENSION" == "tar.gz" ]]; then
    ohai "Extracting..."
    tar -xzf "$TMP_DIR/$DOWNLOAD_FILENAME" -C "$TMP_DIR" || abort "Failed to extract archive"
  elif [[ "$EXTENSION" == "zip" ]]; then
    ohai "Extracting..."
    unzip -q "$TMP_DIR/$DOWNLOAD_FILENAME" -d "$TMP_DIR" || abort "Failed to extract archive"
  else
    abort "Unknown archive format: $EXTENSION"
  fi
}


install_agentuity() {
  ohai "Installing in path $INSTALL_PATH"
  if [[ -f "$TMP_DIR/agentuity" ]]; then
    if is_macos && [[ -f "$INSTALL_PATH/agentuity" ]]; then
      ohai "Removing existing binary to avoid macOS quarantine issues..."
      rm -f "$INSTALL_PATH/agentuity" || abort "Failed to remove existing binary from $INSTALL_PATH"
    fi
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
}

set_path()  {
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
      
      if ! command -v agentuity >/dev/null 2>&1; then
        printf "${tty_blue}==>${tty_bold} ${tty_magenta}To apply changes, restart your terminal or run:${tty_reset} source $SHELL_CONFIG\n"
      fi
      
      export PATH="$PATH:$INSTALL_PATH"
    else
      warn "$INSTALL_PATH is not in your PATH. You may need to add it manually to use the agentuity command."
      case "$SHELL" in
        */bash*)
          echo "  echo '\nexport PATH=\"\$PATH:$INSTALL_PATH\"' >> ~/.bashrc"
          echo "  source ~/.bashrc  # To apply changes immediately"
          ;;
        */zsh*)
          echo "  echo '\nexport PATH=\"\$PATH:$INSTALL_PATH\"' >> ~/.zshrc"
          echo "  source ~/.zshrc  # To apply changes immediately"
          ;;
        */fish*)
          echo "  echo '\nset -gx PATH \$PATH $INSTALL_PATH' >> ~/.config/fish/config.fish"
          echo "  source ~/.config/fish/config.fish  # To apply changes immediately"
          ;;
        *)
          echo "  Add $INSTALL_PATH to your PATH"
          echo "  Then restart your terminal or reload your shell configuration"
          ;;
      esac
    fi
  fi
}

install_completions() {
  if command -v "$INSTALL_PATH/agentuity" >/dev/null 2>&1; then
    COMPLETION_DIR=""
    if is_macos; then
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
      fi

      if is_linux; then
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
      fi
  fi
}

success() {
  ohai "Installation complete! Run 'agentuity --help' to get started."
  ohai "For more information, visit: $(url "https://agentuity.dev")"
  
  if ! command -v agentuity >/dev/null 2>&1; then
    printf "${tty_blue}==>${tty_bold} ${tty_magenta}To apply PATH changes, restart your terminal or run:${tty_reset} source ~/.$(basename $SHELL)rc\n"
  fi
  
  exit 0
}


cleanup() {
  rm -rf "$TMP_DIR"
}

main() {

  abort_if_windows

  debug "1 check_known_arch"
  check_known_arch


  VERSION="latest"
  TMP_DIR="$(mktemp -d)"

  trap cleanup EXIT
  
  debug "2 setup_install_path_var"
  setup_default_install_path_var

  debug "3 parse_cli_args: $@"
  parse_cli_args "$@"

  debug "4 check_install_path"
  check_install_path

  debug "5 check_version"
  check_version


  if is_macos; then
    debug "6 install_mac"
    install_mac
  fi

  debug "7 check_latest_release"
  check_latest_release

  debug "8 download_release"
  download_release

  debug "9 download_checksums"
  download_checksums

  debug "10 extract_release"
  extract_release

  debug "11 install_agentuity"
  install_agentuity

  debug "12 set_path"
  set_path

  debug "13 install_completions"
  install_completions

  debug "14 success"
  success
}

main "$@"
