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
      # Single-token validation: Go module paths must not contain spaces.
      case "${rest}" in
        '' | *[[:space:]]*)
          echo "agent-lsp: ERROR: go package spec must be a single token: ${rest}" >&2
          return 1
          ;;
      esac
      GOPATH=/usr/local go install "${rest}"
      ;;
    "go install "*)
      rest="${cmd#go install }"
      case "${rest}" in
        '' | *[[:space:]]*)
          echo "agent-lsp: ERROR: go package spec must be a single token: ${rest}" >&2
          return 1
          ;;
      esac
      go install "${rest}"
      ;;
    "npm install -g "*)
      rest="${cmd#npm install -g }"
      # npm allows multiple space-separated package names — validate chars but allow spaces.
      case "${rest}" in
        '' | *[!\ a-zA-Z0-9@._/:+-]*)
          echo "agent-lsp: ERROR: npm package spec contains invalid characters: ${rest}" >&2
          return 1
          ;;
      esac
      # Word-split intentional: supports multi-package installs (e.g. typescript-language-server typescript)
      # shellcheck disable=SC2086
      npm install -g ${rest}
      ;;
    "apt-get update"*)
      # Extract the package name from the fixed pattern:
      #   apt-get update && apt-get install -y --no-install-recommends <pkg> && rm -rf ...
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
      case "${rest}" in
        '' | *[[:space:]]*)
          echo "agent-lsp: ERROR: cargo package spec must be a single token: ${rest}" >&2
          return 1
          ;;
      esac
      cargo install "${rest}"
      ;;
    "pip install "*)
      rest="${cmd#pip install }"
      case "${rest}" in
        '' | *[[:space:]]*)
          echo "agent-lsp: ERROR: pip package spec must be a single token: ${rest}" >&2
          return 1
          ;;
      esac
      pip install "${rest}"
      ;;
    "dotnet tool install -g "*)
      rest="${cmd#dotnet tool install -g }"
      case "${rest}" in
        '' | *[[:space:]]*)
          echo "agent-lsp: ERROR: dotnet package spec must be a single token: ${rest}" >&2
          return 1
          ;;
      esac
      dotnet tool install -g "${rest}"
      ;;
    "gem install "*)
      rest="${cmd#gem install }"
      case "${rest}" in
        '' | *[[:space:]]*)
          echo "agent-lsp: ERROR: gem package spec must be a single token: ${rest}" >&2
          return 1
          ;;
      esac
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

  # Split LSP_SERVERS on commas and iterate. Uses while-read to prevent glob
  # expansion from ${servers} (CRITICAL: unquoted for-loop expands * against filesystem).
  printf '%s\n' "${LSP_SERVERS}" | tr ',' '\n' | while IFS= read -r server; do
    # Trim any leading/trailing whitespace
    server=$(printf '%s' "${server}" | tr -d ' \t')
    [ -z "${server}" ] && continue

    # Validate server name against allowlist (MEDIUM-4: prevents path traversal in
    # CACHE_DIR and awk injection). Only a-z, A-Z, 0-9, hyphens, dots, underscores.
    case "${server}" in
      *[!a-zA-Z0-9._-]*)
        echo "agent-lsp: WARNING: invalid server name '${server}' (allowed: a-z A-Z 0-9 . _ -), skipping" >&2
        continue
        ;;
    esac

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
