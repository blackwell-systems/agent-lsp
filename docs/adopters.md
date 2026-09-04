# Who Uses agent-lsp

Projects and developers using agent-lsp as their code-intelligence backend, plus
where it shows up packaged and listed. Using it in something? Add yourself to the
[Who's using agent-lsp? thread](https://github.com/blackwell-systems/agent-lsp/issues)
or email dayna@blackwell-systems.com and we'll pull it in.

## Integrations (agent-lsp as a backend / search layer)

### Clausura

[Clausura](https://github.com/liuyanghejerry/Clausura) is a CI-native agent CLI for
deterministic pipeline gating (202 stars, Rust). It runs agent-lsp as a command backend,
and its example MCP code-review workflow installs agent-lsp (via the official `install.sh`)
plus the relevant language server as a CI prerequisite, using LSP-grade intelligence to
gate pull requests.

### vmcp

[vmcp](https://github.com/hewimetall/vmcp) is an MCP tool gateway with a GraphQL surface
(Rust). It ships a `run-agent-lsp.sh` that launches agent-lsp (MCP over HTTP) against
rust-analyzer for the workspace, and its demo docs instruct users to `pip install agent-lsp`.
An infrastructure-layer integration: agent-lsp as the code-intelligence node behind the gateway.

### autospec

[autospec](https://github.com/berlinguyinca/autospec) is an AI workflow that turns specs
into GitHub issue trees (Rust). agent-lsp is its **v1 code-intelligence backend**, pinned
behind a replaceable abstraction (agent-lsp / ast-grep / ripgrep) so the workflow can
resolve code structure while staying backend-agnostic.

### TDQ-Workflow

[TDQ-Workflow](https://github.com/TDQUOC/TDQ-Workflow) makes agent-lsp **the workflow's
search layer** (Python). It ships a dedicated `tdq-lsp-setup` skill (a seven-rung setup
guide covering binary, MCP server, language servers, permissions, and config) and a
`tdq_lsp.py` diagnostic, so the workflow searches code "by meaning, not by text."

## In developer environments

- **[AllySummers/dotfiles](https://github.com/AllySummers/dotfiles)** installs agent-lsp
  via `mise` (`tools."github:blackwell-systems/agent-lsp" = "latest"`) with a dedicated
  `agent-lsp-mcp` task, running it as a live MCP tool in the dev environment.
- **[aashutosh396/mindpalace](https://github.com/aashutosh396/mindpalace)** vendors the
  agent-lsp skill into a personal skill collection.
- **[yechua-silva/zyrocli](https://github.com/yechua-silva/zyrocli)** evaluates agent-lsp
  as a code-intelligence layer (65 tools, 30 languages), calling it "revolucionario para Go."

## Listed & packaged

- Listed across community catalogs: **awesome-mcp-servers** (TensorBlock),
  **awesome-claude-skills**, **awesome-cli-coding-agents**, **awesome-devops-mcp-servers**,
  **Awesome-MCP-ZH**, and the **mcp-servers-live** registry.
- **NUR (Nix User Repository)**: agent-lsp packaged and installable across NixOS.
- Published to the official MCP Registry, Glama (A-tier), Smithery, and Winget.

## In the wild

- A third-party evaluation scorecard benchmarks agent-lsp head-to-head against Serena,
  summarizing the trade-off as "Serena for maturity; agent-lsp if the warm index and the
  CI-verified language support matrix are what you're buying."
