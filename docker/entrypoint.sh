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

# run_install_command: execute a validated install command.
# Only commands matching known package manager prefixes are allowed.
# Any unrecognized command is rejected with an error (not silently skipped).
run_install_command() {
  cmd="$1"
  case "${cmd}" in
    "GOPATH=/usr/local go install "*)
      rest="${cmd#GOPATH=/usr/local go install }"
      GOPATH=/usr/local go install "${rest}"
      ;;
    "go install "*)
      rest="${cmd#go install }"
      go install "${rest}"
      ;;
    "npm install -g "*)
      rest="${cmd#npm install -g }"
      npm install -g "${rest}"
      ;;
    "apt-get update"*)
      # Extract the package name from the fixed pattern:
      #   apt-get update && apt-get install -y --no-install-recommends <pkg> && rm -rf ...
      # Use sed to capture the token after --no-install-recommends up to the next space or end.
      pkg=$(printf '%s' "${cmd}" | sed 's/.*--no-install-recommends \([^ ]*\).*/\1/')
      # Validate: only alphanumeric, hyphens, dots, plus, underscores — no shell metacharacters.
      case "${pkg}" in
        '' | *[!a-zA-Z0-9._+\-]*)
          echo "agent-lsp: ERROR: invalid apt package name '${pkg}'" >&2
          return 1
          ;;
      esac
      apt-get update && apt-get install -y --no-install-recommends "${pkg}" && rm -rf /var/lib/apt/lists/*
      ;;
    "cargo install "*)
      rest="${cmd#cargo install }"
      cargo install "${rest}"
      ;;
    "pip install "*)
      rest="${cmd#pip install }"
      pip install "${rest}"
      ;;
    "dotnet tool install -g "*)
      rest="${cmd#dotnet tool install -g }"
      dotnet tool install -g "${rest}"
      ;;
    "gem install "*)
      rest="${cmd#gem install }"
      gem install "${rest}"
      ;;
    "echo "*)
      # Manual install: no automated installation available — print the instruction.
      printf 'agent-lsp: manual install required: %s\n' "${cmd#echo }" >&2
      ;;
    *)
      echo "agent-lsp: ERROR: unrecognized install command prefix, refusing to execute: ${cmd}" >&2
      return 1
      ;;
  esac
}

# get_install_command: extract install_command for a given server name from the YAML registry.
# Uses awk -v for variable binding to prevent server_name from being interpreted as a regex.
get_install_command() {
  server_name="$1"
  awk -v name="${server_name}" '
    $0 == "  - name: " name { found=1; next }
    found && /install_command:/ { sub(/.*install_command: *"/, ""); sub(/"$/, ""); print; exit }
    found && /^  - name:/ { exit }
  ' "${REGISTRY}"
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
    run_install_command "${install_cmd}"

    # Create cache marker so future container starts skip the install
    mkdir -p "${CACHE_DIR}/${server}"
    touch "${CACHE_DIR}/${server}/.installed"
    echo "agent-lsp: ${server} installed and cached"
  done
fi

exec /usr/local/bin/agent-lsp "$@"
