#!/bin/bash
# Publish pre-built wheels to PyPI.
# Run after pypi-build-wheels.sh has populated pypi/dist/.
#
# Usage: ./scripts/pypi-publish.sh
#
# Requires: twine, TWINE_USERNAME + TWINE_PASSWORD env vars (or ~/.pypirc)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/../pypi/dist"

if [ ! -d "$DIST_DIR" ] || [ -z "$(ls "$DIST_DIR"/*.whl 2>/dev/null)" ]; then
  echo "No wheels found in ${DIST_DIR}. Run pypi-build-wheels.sh first."
  exit 1
fi

echo "Publishing to PyPI:"
ls -1 "$DIST_DIR"/*.whl

twine upload "$DIST_DIR"/*.whl

echo "Done. View at https://pypi.org/project/agent-lsp/"
