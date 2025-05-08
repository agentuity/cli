#!/bin/bash
set -e

chmod +x ./install.sh

echo "####################################################################################################"
echo "### Test Install without arguments (latest version and default install path)"
./install.sh 



# Verify installation
LATEST_VERSION=$(/root/.local/bin/agentuity --version)
echo "Installed version: $LATEST_VERSION"
echo "####################################################################################################"
echo ""
echo ""
echo "####################################################################################################"
echo "### Test Install with version"
./install.sh -v 0.0.118

# Verify installation
CURRENT_VERSION=$(/root/.local/bin/agentuity --version)
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

