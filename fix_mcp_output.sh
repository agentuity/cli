#!/bin/bash
# This script wraps the agentuity mcp command and fixes the output format
# by removing the redundant "agentuity" prefix from the manual installation path

# Run the original command and capture its output
output=$(/home/ubuntu/repos/cli/agentuity "$@")

# Fix the output by removing the "agentuity" prefix from the manual installation path
echo "$output" | sed 's/^  agentuity /  /'
