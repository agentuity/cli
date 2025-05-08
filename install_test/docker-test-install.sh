#!/bin/bash
set -e

chmod +x ./install.sh

# Same logic as install.sh
if [ -d "$HOME/.local/bin" ] && [ -w "$HOME/.local/bin" ]; then
  INSTALL_PATH="$HOME/.local/bin"
elif [ -d "$HOME/.bin" ] && [ -w "$HOME/.bin" ]; then
  INSTALL_PATH="$HOME/.bin"
elif [ -d "$HOME/bin" ] && [ -w "$HOME/bin" ]; then
  INSTALL_PATH="$HOME/bin"
else
    if [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
      if [ -w "$HOME/.local/bin" ]; then
        INSTALL_PATH="$HOME/.local/bin"
      fi
    elif [ -d "$HOME/.bin" ] || mkdir -p "$HOME/.bin" 2>/dev/null; then
      if [ -w "$HOME/.bin" ]; then
        INSTALL_PATH="$HOME/.bin"
      fi
    elif [ -d "$HOME/bin" ] || mkdir -p "$HOME/bin" 2>/dev/null; then
      if [ -w "$HOME/bin" ]; then
        INSTALL_PATH="$HOME/bin"
      fi
    else
      abort "Could not find or create a writable installation directory. Try running with sudo or specify a different directory with --dir."
    fi
fi

export PATH="$INSTALL_PATH:$PATH"

echo "####################################################################################################"
echo "### Test Install without arguments (latest version and default install path)"
./install.sh 

# Verify installation
LATEST_VERSION=$(agentuity --version)
echo "Installed version: $LATEST_VERSION"
echo "####################################################################################################"
echo ""
echo ""
echo "####################################################################################################"
echo "### Test Install with version"
./install.sh -v 0.0.118

# Verify installation
CURRENT_VERSION=$(agentuity --version)
echo "Installed version (specific): $LATEST_VERSION"
if [ "$CURRENT_VERSION" != "0.0.118" ]; then
  echo "Version verification failed"
  exit 1
fi
echo "####################################################################################################"
echo ""
echo ""
echo "####################################################################################################"
echo "### Test Install with directory and version"
./install.sh -d /tmp/agentuity-test-version -v 0.0.118
CURRENT_VERSION=$(/tmp/agentuity-test-version/agentuity --version)
if [ "$CURRENT_VERSION" != "0.0.118" ]; then
  echo "Version verification failed"
  exit 1
fi
echo "####################################################################################################"

