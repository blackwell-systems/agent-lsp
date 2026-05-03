"""Entry point for `python -m agent_lsp` and the `agent-lsp` console script."""

import os
import sys
import subprocess


def _find_binary():
    """Locate the agent-lsp binary bundled in this package."""
    pkg_dir = os.path.dirname(os.path.abspath(__file__))
    names = ["agent-lsp.exe", "agent-lsp"] if sys.platform == "win32" else ["agent-lsp"]
    for name in names:
        path = os.path.join(pkg_dir, "bin", name)
        if os.path.isfile(path):
            return path
    return None


def main():
    binary = _find_binary()
    if binary is None:
        print(
            "agent-lsp: binary not found. This platform may not be supported.\n"
            "Install from https://github.com/blackwell-systems/agent-lsp/releases",
            file=sys.stderr,
        )
        sys.exit(1)

    result = subprocess.run([binary] + sys.argv[1:])
    sys.exit(result.returncode)


if __name__ == "__main__":
    main()
