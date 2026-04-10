#!/bin/sh
# agent-lsp entrypoint
# Reads LSP_SERVERS env var (comma-separated server names) and installs
# any requested language servers not already present, using the registry
# at /etc/agent-lsp/lsp-servers.yaml. Caches installs to /var/cache/lsp-servers/.
# Then exec's agent-lsp with all passed arguments.
#
# NOTE: The Dockerfile must include:
#   COPY docker/lsp-servers.yaml /etc/agent-lsp/lsp-servers.yaml
set -e

CACHE_DIR=/var/cache/lsp-servers
REGISTRY=/etc/agent-lsp/lsp-servers.yaml

# get_install_command: extract install_command for a given server name from the YAML registry.
# Uses only grep/awk/sed — no yq, python, or jq required.
get_install_command() {
  server_name="$1"
  # Match the block starting at "  - name: <server_name>" and ending at the next "  - name:" entry.
  # Extract the first install_command value found in that block.
  awk "/^  - name: ${server_name}$/,/^  - name:/" "${REGISTRY}" \
    | grep "install_command:" \
    | head -1 \
    | sed 's/.*install_command: *"\(.*\)"/\1/'
}

if [ -n "${LSP_SERVERS}" ]; then
  echo "agent-lsp: installing requested language servers: ${LSP_SERVERS}"
  mkdir -p "${CACHE_DIR}"

  # Split LSP_SERVERS on commas (POSIX sh compatible, no bashisms)
  servers=$(printf '%s\n' "${LSP_SERVERS}" | tr ',' '\n')

  for server in ${servers}; do
    # Trim any leading/trailing whitespace (use tr to strip spaces/tabs)
    server=$(printf '%s' "${server}" | tr -d ' \t')
    [ -z "${server}" ] && continue

    # Check if already on PATH
    if command -v "${server}" > /dev/null 2>&1; then
      echo "agent-lsp: ${server} already on PATH, skipping"
      continue
    fi

    # Check cache marker
    if [ -f "${CACHE_DIR}/${server}/.installed" ]; then
      echo "agent-lsp: ${server} already cached, skipping"
      continue
    fi

    # Look up install command in registry
    install_cmd=$(get_install_command "${server}")

    if [ -z "${install_cmd}" ]; then
      echo "agent-lsp: WARNING: no registry entry found for '${server}', skipping" >&2
      continue
    fi

    echo "agent-lsp: installing ${server}: ${install_cmd}"
    eval "${install_cmd}"

    # Create cache marker so future container starts skip the install
    mkdir -p "${CACHE_DIR}/${server}"
    touch "${CACHE_DIR}/${server}/.installed"
    echo "agent-lsp: ${server} installed and cached"
  done
fi

exec /usr/local/bin/agent-lsp "$@"
