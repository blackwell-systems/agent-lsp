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

## Research and training datasets

### NVIDIA Research Skill2Env

[NVIDIA Research's Skill2Env](https://github.com/NVlabs/Skill2Env) turns public
Agent Skills into runnable terminal exercises for reinforcement learning. Each
exercise gives an agent a task, a container with the workspace and tools, and
tests that score the result. The [technical report](https://www.alphaxiv.org/abs/2609.reinforcing-agents-collective-skills)
describes how NVIDIA generated these environments and used them to train a
terminal agent.

**From 24 source workflows to 41 released tasks.** NVIDIA's public
[SkillHub source inventory](https://github.com/NVlabs/Skill2Env/blob/main/SkillHub/skillhub_source_licenses.csv)
credits agent-lsp for 24 of its 2,396 entries. The
[public Harbor dataset](https://hub.harborframework.com/datasets/skill2env/skill2env)
lists 41 tasks named for those workflows out of 7,496 released tasks. They
include four refactoring exercises, four code-generation exercises, and two
speculative-editing exercises, alongside tasks for cross-repository navigation,
impact analysis, and other workflows.

**What carried over.** The upstream
[`lsp-simulate` skill](https://github.com/NVlabs/Skill2Env/blob/main/SkillHub/skills/lsp-simulate/SKILL.md)
teaches an agent to stage edits in memory and inspect diagnostics before
writing. The released
[Chronos](https://hub.harborframework.com/tasks/skill2env/task_lsp-simulate_62fz7nkp)
and [Beacon](https://hub.harborframework.com/tasks/skill2env/task_lsp-simulate_gdmaapw9)
exercises put that procedure into concrete Go workspaces with task-local
simulators. The Dockerfiles for the 41 released exercises show no explicit
installation of the upstream agent-lsp MCP server. The verified contribution
is the set of workflows used to design the exercises.

The [project writeup](https://yifzhang.com/blog/Skill2Env/#corpus) describes the
public Harbor release as a separate generation from the experimental corpus.
The 41-task count refers to the public release. The report does not identify
which agent-lsp-derived tasks were included in the reported training runs.

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
