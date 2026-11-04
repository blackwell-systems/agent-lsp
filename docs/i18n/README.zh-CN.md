[English](../../README.md) · **简体中文** · [Русский](README.ru.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

<p align="center">
  <img src="../../assets/banner.png" alt="agent-lsp" width="820">
</p>

<p align="center">
  <a href="#tools"><img src="https://img.shields.io/badge/CI--verified_tools-66%2F66-brightgreen.svg" alt="CI Coverage"></a>
  <a href="#multi-language-support"><img src="https://img.shields.io/badge/languages-32_CI--verified-brightgreen.svg" alt="Languages"></a>
  <a href="https://github.com/blackwell-systems/mcp-assert"><img src="https://raw.githubusercontent.com/blackwell-systems/mcp-assert/main/assets/badge-passing.svg?v=3" alt="mcp-assert: passing" height="20"></a>
  <a href="https://agentskills.io"><img src="../../assets/badge-agentskills.svg" alt="Agent Skills"></a>
  <a href="https://github.com/blackwell-systems/agent-lsp"><img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/blackwell-systems/agent-lsp/badges/assets/downloads-badge.json" alt="downloads"></a>
  <br>
  <a href="https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/"><img src="https://img.shields.io/badge/LSP-3.17-blue.svg" alt="LSP 3.17"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/punkpeye/awesome-mcp-servers"><img src="https://img.shields.io/badge/Awesome-MCP%20Servers-fc60a8" alt="Awesome MCP Servers"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

**面向 AI 智能体的代码智能基础设施。** 66 个工具，32 种经 CI 验证的语言，24 个智能体工作流。单个 Go 二进制文件。

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh && agent-lsp init
```

## 它是什么？

agent-lsp 是一个 **MCP 服务器**，它将现有的 LSP 服务器（gopls、rust-analyzer、jdtls 等）编排成智能体原生的工作流。

**它不是 LSP 服务器** — 而是一个编排层，负责管理语言服务器，并通过 MCP 工具暴露批量操作、推测式编辑和多步骤工作流。

**架构：**
- **语言服务器**（gopls、rust-analyzer 等）→ 提供代码智能
- **agent-lsp**（MCP 服务器）→ 编排工作流，维持热运行时
- **AI 智能体** → 通过 MCP 协议进行消费

## 为什么选择 agent-lsp？

**持久的热运行时**  
语言服务器在多个智能体会话之间保持已建立索引的状态。首次会话：为工作区建立索引（对于典型项目约 10 秒）。后续会话：即时可用。每次请求不再有冷启动开销。

**批量操作**  
`blast_radius` → 一次调用即返回所有导出符号及其所有调用方（按测试与非测试进行划分）。若没有编排：需要 20 多次连续的 LSP 调用。

**推测式编辑**  
`simulate_edit` → 在内存中预览更改，检查诊断增量，然后应用或丢弃。在触及磁盘之前先测试编辑。

**工作流编排**  
24 个技能将 LSP 操作串联成完整的流水线：
- `/lsp-refactor` → 影响分析 → 预览 → 应用 → 验证构建 → 运行测试
- `/lsp-safe-edit` → 预览 → 诊断差异 → 若安全则应用
- `/lsp-verify` → LSP 诊断 → 构建 → 测试套件

**多语言，单会话**  
一个 agent-lsp 进程将 `.go` 路由到 gopls，将 `.ts` 路由到 tsserver，将 `.py` 路由到 pyright。在项目之间无需重新配置。会话跨文件和跨仓库持续保持。

> [!TIP]
> **Token 优化输出：** 工具响应以 [GCF](https://gcformat.com) 而非 JSON 编码。视工具不同，可减少 30-84% 的 token（配合会话去重最高可达 92.7%）。[在每个前沿模型上均达到 100% 的 LLM 理解率](https://gcformat.com/guide/benchmarks.html)，在复杂代码图上达到 91.2%，而 JSON 在此类场景中平均仅为 54.1%。参见[下文](#token-optimized-output-gcf)了解各工具实测的节省情况。

**各部分如何协同：** [LSP](https://microsoft.github.io/language-server-protocol/)（语言服务器协议）是编辑器获取代码智能的方式：补全、诊断、跳转到定义。[MCP](https://modelcontextprotocol.io/)（模型上下文协议）是 Claude Code 等 AI 工具发现和调用外部工具的标准方式。agent-lsp 将二者桥接起来：让语言服务器的智能能力可供 AI 智能体使用。

## 何时使用

- 构建智能体式代码生成系统
- 在大型代码库中自动化重构
- 需要以编程方式获取代码智能的 CI 工具
- 任何顺序 LSP 调用过慢或过于复杂的工作流

### 智能体的评价

我们请 AI 智能体在 10 项编码任务（查找调用方、安全重命名、预览编辑、检测死代码）中评估 agent-lsp，并写出诚实的评估意见。四个不同的模型，四次独立的评估，得出相同的结论：

> **Claude (Opus 4.6)：** “对于任何涉及重构、影响分析或安全编辑的工作流，我都会推荐 agent-lsp。最出色的工具是 `blast_radius`（一次调用即可获得影响范围，并带有测试/非测试划分，若用 grep 复现则需要 5-10 条命令）、`go_to_implementation`（经类型检查的接口满足关系，这是 grep 根本做不到的），以及仿真会话工作流（无需触及磁盘的推测式类型检查，这在 grep/read 中完全没有等价物）。”

> **Cursor (auto)：** “对于繁重的重构和代码导航，我会推荐 agent-lsp，因为它的重命名、引用、实现、调用层次和仿真工具消除了大量脆弱的 grep/手动编辑工作，使更改更安全。”

> **GPT-5.5 (通过 Codex)：** “对于符号感知型工作，我会推荐 agent-lsp：引用、实现、重命名预览、诊断以及大文件结构分析，都比 grep/read 循环明显更快、更不易出错。”

> **Gemini 2.5 Pro (通过 Gemini CLI)：** “我强烈推荐 agent-lsp，因为它提供了标准文本搜索工具根本无法匹敌的语义感知层级。无需写入磁盘即可执行高置信度的重命名、查找接口实现以及预览编辑的诊断影响，这大大降低了引入回归的风险。”

### 经过测试，而非假设

其他所有的 MCP-LSP 实现都在配置文件中列出支持的语言。它们中没有一个会在 CI 中运行实际的语言服务器来验证其是否真正可用。

agent-lsp 的 CI 在每次推送时都会针对真实的固定装置代码库运行 **32 个真实的语言服务器**：Go、Python、TypeScript、Rust、Java、C、C++、C#、Ruby、PHP、Kotlin、Swift、Scala、Zig、Lua、Elixir、Gleam、Clojure、Dart、Terraform、Nix、Prisma、SQL、MongoDB、MQL 等。当我们说“可与 gopls 配合工作”时，这是一个经过验证的、自动化的论断，而非一种期望。

### 推测式执行

在写入磁盘之前先在内存中模拟更改。没有其他 MCP-LSP 实现具备此能力。

`preview_edit` 预览任何编辑的诊断影响。你可以在文件被触及之前精确看到什么会被破坏。`simulate_chain` 评估一系列相互依赖的编辑（重命名一个函数、更新所有调用方、更改返回类型），并报告哪一步首先引入了错误。

8 个推测式执行工具。完整工作流参见 [docs/guide/speculative-execution.md](../../docs/guide/speculative-execution.md)。

### Token 节省

在相同任务上，结构化的 LSP 响应比 grep/read 使用的 token **少 5-34 倍**。在 HashiCorp Consul（31.9 万行代码）上，一次影响范围分析通过 grep 使用 17.7MB，而通过 LSP 仅使用 841KB，将 5,534 次工具调用减少到 119 次。节省效果随代码库规模而扩大。完整实验（涵盖五个代码库）参见 [docs/guide/token-savings.md](../../docs/guide/token-savings.md)。

### Token 优化输出（GCF）

工具响应以 [GCF (Graph Compact Format)](https://gcformat.com) 而非 JSON 编码。GCF 消除了字段名重复、标识符重复以及每条记录的结构开销。

| 配置 | 工具 | 相对 JSON 的节省 |
|---------|-------|----------------|
| Tabular | find_references、get_diagnostics、detect_changes 等 | **30-51%** |
| Graph | list_symbols、find_symbol、blast_radius、find_callers、explore_symbol、type_hierarchy、get_cross_repo_references | **79-84%** |
| Graph + 会话去重 | 相同，通过 [gcf-proxy](https://github.com/blackwell-systems/gcf-proxy) `--session` | **92.7%**（第 5 次调用） |

分组/嵌套的响应（符号下的调用方、带相关信息的诊断）也会被表格化，在该形态下相对 JSON 约节省 14%（[详情](../../docs/guide/gcf-integration.md#nested-container-responses-grouped-data)）。

GCF 默认启用。若要还原为 JSON：

```bash
export AGENT_LSP_OUTPUT_FORMAT=json
```

基准测试：`go run scripts/gcf-benchmark.go`。架构细节参见 [docs/guide/gcf-integration.md](../../docs/guide/gcf-integration.md)。

**GCF：** [gcformat.com](https://gcformat.com) · [Spec](https://github.com/blackwell-systems/gcf) · [Go](https://github.com/blackwell-systems/gcf-go) · [Python](https://github.com/blackwell-systems/gcf-python) · [TypeScript](https://github.com/blackwell-systems/gcf-typescript) · [Playground](https://gcformat.com/playground.html)

### 为什么编排很重要

AI 智能体做出错误的代码更改，是因为它们看不到全局：谁调用了这个函数、如果我重命名它会破坏什么、构建是否仍然通过。语言服务器掌握着答案，但原始的 LSP 工具需要 20 多次连续调用和复杂的编排逻辑。

agent-lsp 通过将正确的多步骤操作编码为单次调用和技能来解决这个问题。`blast_radius` 用一次调用完成智能体本需 20 多次调用才能完成的工作。`/lsp-refactor` 将影响分析 → 预览 → 应用 → 验证 → 测试串联起来，无需逐提示的编排。

### 持久守护进程模式

Python 和 TypeScript 项目在 `find_references` 可用之前需要数分钟的后台索引。agent-lsp 会自动派生一个持久的守护进程代理，它在会话之间存续，使工作区保持已建立索引的状态。首次会话：守护进程启动并建立索引（FastAPI 约 10 秒）。后续会话：即时连接到热守护进程。在 30 分钟无活动后自动退出。Go、Rust 及其他快速索引的语言完全绕过此机制（零开销）。

### 阶段强制

技能告知智能体正确的操作顺序。阶段强制让运行时*阻止*违规操作，而不是信任智能体去遵循指令。

当智能体激活一个技能时，每一次工具调用都会根据当前阶段的权限进行检查。在影响范围分析期间调用 `apply_edit` 不会悄然继续；它会返回一个带有具体恢复指引的错误（`Recovery: apply_edit becomes available in the "apply" phase. Next phase is "speculative_preview". Allowed tools: [open_document get_diagnostics preview_edit simulate_chain]`）。随着智能体调用后续阶段的工具，阶段会自动推进。

没有其他 MCP 工具提供方在运行时强制工作流顺序。参见 [docs/guide/phase-enforcement.md](../../docs/guide/phase-enforcement.md)。

### 并发分析

检查器包含 4 项并发检查，可跨 25 种语言、4 个并发家族（goroutine、thread、async、actor）工作：

- **未恢复的并发入口**：没有恢复机制的 goroutine/thread/task
- **未检查的共享状态**：对 sync.Map、ConcurrentHashMap 的裸类型断言
- **通道从未关闭**：创建但从未关闭的通道/队列（goroutine 泄漏）
- **无同步的共享字段**：从并发上下文访问但未加同步的字段

当父类型持有互斥锁时，`blast_radius` 会用 `sync_guarded: true` 标注符号。带 `cross_concurrent: true` 的 `find_callers` 会追踪穿越 goroutine/thread 边界的调用链。`/lsp-concurrency-audit` 技能可为任何类型生成字段级的安全报告。

### 自动诊断

符号编辑工具（`replace_symbol_body`、`insert_after_symbol`、`insert_before_symbol`、`safe_delete_symbol`）会自动返回 `errors_after` 和 `warnings_after` 计数。智能体无需单独调用 `get_diagnostics` 即可立即知晓某次编辑是否破坏了什么。

`safe_apply_edit` 将预览与应用合并为一次调用：先进行推测式预览，仅当 `net_delta == 0`（没有新错误）时才应用到磁盘。一次工具调用替代三次。

### 兼容对象

| AI 工具 | 传输方式 | 设置 |
|---------|-----------|-------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | stdio | `agent-lsp init` |
| [Cursor](https://cursor.com) | stdio | `agent-lsp init` |
| [Windsurf](https://windsurf.com) | stdio | `agent-lsp init` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | stdio | `agent-lsp init` |
| [Continue](https://continue.dev) | stdio | [mcp-clients.md](../getting-started/mcp-clients.md) |
| [Cline](https://github.com/cline/cline) | stdio | `agent-lsp init` |
| 任何 MCP 客户端 | Streamable HTTP | `AGENT_LSP_TOKEN=secret agent-lsp --http --port 8080` |

复制粘贴即用的配置参见 [docs/getting-started/mcp-clients.md](../../docs/getting-started/mcp-clients.md)。

## 技能

原始工具会被忽略。技能会被使用。每个技能都编码了正确的工具序列，使工作流无需逐提示的编排指令就能真正发生。技能既作为 [AgentSkills](https://github.com/anthropics/agent-skills) 斜杠命令提供，也作为 MCP 提示通过 `prompts/list` / `prompts/get` 供任何 MCP 客户端使用。

完整描述和使用指引参见 [docs/guide/skills.md](../../docs/guide/skills.md)。

**在你更改任何内容之前**

| 技能 | 用途 |
|-------|---------|
| `/lsp-impact` | 触及某个符号或文件之前的影响范围分析 |
| `/lsp-implement` | 查找某个接口的所有具体实现 |
| `/lsp-dead-code` | 在清理前检测零引用的导出符号 |

**安全编辑**

| 技能 | 用途 |
|-------|---------|
| `/lsp-safe-edit` | 磁盘写入前的推测式预览；前后诊断差异；在错误上呈现代码操作 |
| `/lsp-simulate` | 在内存中测试更改而不触及文件 |
| `/lsp-edit-symbol` | 在不知道文件或位置的情况下编辑命名符号 |
| `/lsp-edit-export` | 安全编辑导出符号，先查找所有调用方 |
| `/lsp-rename` | `prepare_rename` 安全门、预览所有位置、确认、原子应用 |

**入门**

| 技能 | 用途 |
|-------|---------|
| `/lsp-architecture` | Structural architecture overview: languages, package map, entry points, dependency graph, and hotspots |
| `/lsp-onboard` | 首次会话的项目上手：检测语言、映射包、查找入口点和热点、检查诊断 |

**理解不熟悉的代码**

| 技能 | 用途 |
|-------|---------|
| `/lsp-explore` | “告诉我关于这个符号的信息”：悬停 + 实现 + 调用层次 + 引用，一次完成 |
| `/lsp-understand` | 针对某个符号或文件的深入代码地图：类型信息、调用层次、引用、源码 |
| `/lsp-docs` | 三层文档：悬停 → 离线工具链 → 源码 |
| `/lsp-cross-repo` | 在使用方仓库中查找某个库符号的所有用法 |
| `/lsp-local-symbols` | 文件范围的符号列表、用法搜索和类型信息 |

**编辑之后**

| 技能 | 用途 |
|-------|---------|
| `/lsp-verify` | 每次编辑后的诊断 + 构建 + 测试 |
| `/lsp-fix-all` | 为文件中的所有诊断应用快速修复代码操作 |
| `/lsp-test-correlation` | 查找并仅运行覆盖已编辑文件的测试 |
| `/lsp-format-code` | 通过语言服务器格式化程序格式化文件或选区 |

**生成代码**

| 技能 | 用途 |
|-------|---------|
| `/lsp-generate` | 触发服务端代码生成（接口存根、测试骨架、模拟对象） |
| `/lsp-extract-function` | 通过代码操作将代码块提取为命名函数 |

**完整工作流**

| 技能 | 用途 |
|-------|---------|
| `/lsp-refactor` | 端到端重构：影响范围 → 预览 → 应用 → 验证 → 测试 |
| `/lsp-inspect` | 完整代码质量审计（12 项检查）：死符号、测试覆盖率、错误处理、文档漂移、并发安全 |
| `/lsp-concurrency-audit` | 针对某个类型的字段级并发安全审计：追踪并发访问，标记未同步的字段 |

## Docker

**Stdio 模式**（MCP 客户端直接派生容器）：

```bash
# Go
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:go go:gopls

# TypeScript
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:typescript typescript:typescript-language-server,--stdio

# Python
docker run --rm -i -v /your/project:/workspace ghcr.io/blackwell-systems/agent-lsp:python python:pyright-langserver,--stdio
```

**HTTP 模式**（持久服务，远程客户端通过 Streamable HTTP 连接）：

```bash
docker run --rm \
  -p 8080:8080 \
  -v /your/project:/workspace \
  -e AGENT_LSP_TOKEN=your-secret-token \
  ghcr.io/blackwell-systems/agent-lsp:go \
  --http --port 8080 --listen-addr 0.0.0.0 go:gopls
```

镜像默认以非 root 用户（uid 65532）运行。请通过环境变量设置 `AGENT_LSP_TOKEN`，切勿在命令行上使用 `--token`。镜像同时镜像到 Docker Hub（`blackwellsystems/agent-lsp`）。完整标签列表、HTTP 模式设置和安全加固选项参见 [DOCKER.md](../../DOCKER.md)。

## 设置

### 第 1 步：安装 agent-lsp

```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.sh | sh
```

<details>
<summary>其他安装方法</summary>

**macOS / Linux**

```bash
brew install blackwell-systems/tap/agent-lsp
```

**Windows**

```powershell
# PowerShell (no admin required)
iwr -useb https://raw.githubusercontent.com/blackwell-systems/agent-lsp/main/install.ps1 | iex

# Scoop
scoop bucket add blackwell-systems https://github.com/blackwell-systems/agent-lsp
scoop install blackwell-systems/agent-lsp

# Winget
winget install BlackwellSystems.agent-lsp
```

**所有平台**

```bash
# pip
pip install agent-lsp

# npm
npm install -g @blackwell-systems/agent-lsp

# Go install
go install github.com/blackwell-systems/agent-lsp/cmd/agent-lsp@latest
```

</details>

### 第 2 步：安装语言服务器

为你的技术栈安装相应的服务器。常见的有：

| 语言 | 服务器 | 安装 |
|----------|--------|---------|
| TypeScript / JavaScript | `typescript-language-server` | `npm i -g typescript-language-server typescript@6` |
| Python | `pyright-langserver` | `npm i -g pyright` |
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rust-analyzer` | `rustup component add rust-analyzer` |
| C / C++ | `clangd` | `apt install clangd` / `brew install llvm` |
| Ruby | `solargraph` | `gem install solargraph` |

支持语言及其 CI 状态的完整列表参见 [docs/reference/language-support.md](../../docs/reference/language-support.md)。

### 第 3 步：验证设置

```bash
agent-lsp doctor
```

探测每个已配置的语言服务器并报告其能力。在继续之前修复任何失败项。安装命令和服务器特定说明参见[语言支持](../../docs/reference/language-support.md)。

### 第 4 步：配置你的 AI 工具

```bash
agent-lsp init
```

检测你 PATH 上的语言服务器，询问你使用哪个 AI 工具，写入正确的 MCP 配置，并为你的 AI 提供方安装技能感知规则（Claude Code 用 CLAUDE.md，Cursor 用 `.cursor/rules/`，Cline 用 `.clinerules`，Windsurf 用 `.windsurfrules`，Gemini CLI 用 `GEMINI.md`）。用于 CI 或脚本化场景：`agent-lsp init --non-interactive`。

生成的配置形如：

```json
{
  "mcpServers": {
    "agent-lsp": {
      "type": "stdio",
      "command": "agent-lsp",
      "args": [
        "go:gopls",
        "typescript:typescript-language-server,--stdio",
        "python:pyright-langserver,--stdio"
      ]
    }
  }
}
```

每个参数为 `language:server-binary`（服务器参数以逗号分隔）。

### 第 5 步：安装技能

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git /tmp/agent-lsp-skills
cd /tmp/agent-lsp-skills/skills && ./install.sh --copy
```

技能是复制到你 AI 工具配置中的提示文件。`--copy` 意味着之后可以安全地删除该克隆。

技能同样作为 **MCP 提示**提供：任何 MCP 客户端都可以通过 `prompts/list` 发现它们，并通过 `prompts/get` 获取完整的工作流指令，无需手动安装。`install.sh` 这条路径适用于兼容 AgentSkills 的客户端（Claude Code 斜杠命令）。

### 第 6 步：允许工具权限（Claude Code）

对于 Claude Code，将 `mcp__agent-lsp__*` 添加到你的权限允许列表，使全部 66 个工具无需逐工具的批准提示即可使用：

```json
// ~/.claude/settings.json
{
  "permissions": {
    "allow": ["mcp__agent-lsp__*"]
  }
}
```

若没有这一项，Claude Code 会在每次工具调用时请求权限。其他 MCP 客户端处理权限的方式不同；请查阅你客户端的文档。

技能是编码了可靠流程的多工具工作流：编辑前的影响范围检查、写入前的推测式预览、更改后的测试运行。完整列表参见 [docs/guide/skills.md](../../docs/guide/skills.md)。

### 第 7 步：开始工作

你的 AI 智能体会自动调用工具。第一次调用会初始化工作区：

```
start_lsp(root_dir="/your/project")
```

这是智能体所做的事，而非你需要输入的内容。然后即可使用全部 66 个工具中的任意一个。会话保持热状态；切换文件时无需重启。

## agent-lsp 的独特之处

| 能力 | 详情 |
|------------|---------|
| 工具 | **66** |
| 语言（经 CI 验证） | **32**，每次推送均进行端到端集成测试 |
| 智能体工作流（技能） | **24**，命名的多步骤流程，可通过 MCP `prompts/list` 发现 |
| 推测式执行 | **8 个工具**，在写入磁盘前模拟更改 |
| 阶段强制 | **4 个技能**，运行时阻止乱序的工具调用并给出恢复指引 |
| 连接模型 | **持久**，跨文件和跨项目的热索引 |
| 调用层次 | **✓**，单个工具，带方向参数 |
| 类型层次 | **✓**，经 CI 验证 |
| 跨仓库引用 | **✓**，多根工作区 |
| 自动监听 | **✓**，始终开启、去抖的文件监听 |
| Streamable HTTP 传输 | **✓**，bearer token 鉴权，非 root Docker |
| 分发 | **单个 Go 二进制文件**，10 个安装渠道 |

## 使用场景

- **多项目会话**：将你的 AI 指向 `~/code/`，无需重新配置即可跨任意项目工作
- **多语种开发**：Go 后端 + TypeScript 前端 + Python 脚本，同处一个会话
- **大型 monorepo**：一个服务器处理所有语言，按文件扩展名路由
- **代码迁移**：以完整的跨仓库引用追踪跨仓库重构
- **CI 流水线**：针对真实的语言服务器行为进行验证
- **小众语言栈**：Gleam、Elixir、Prisma、Zig、Clojure、Nix、Dart、Scala、MongoDB，全部经 CI 验证

## 多语言支持

32 种语言在每次 CI 运行时都针对真实的语言服务器进行端到端的 CI 验证。没有其他 MCP-LSP 实现在 CI 中测试哪怕一种语言。

Go、Python、TypeScript、Rust、Java、C、C++、C#、Ruby、PHP、Kotlin、Swift、Scala、Zig、Lua、Elixir、Gleam、Clojure、Dart、Terraform、Nix、Prisma、SQL、MongoDB、JavaScript、YAML、JSON、Dockerfile、CSS、HTML、MQL、Luau。

完整的覆盖矩阵参见 [docs/reference/language-support.md](../../docs/reference/language-support.md)。

## 工具

66 个工具，涵盖导航、分析、重构、符号编辑、组合式探索、安全编辑、推测式执行以及会话生命周期。全部经 CI 验证。

带参数和示例的完整参考参见 [docs/reference/tools.md](../../docs/reference/tools.md)。

## 延伸阅读

### 文档

- [工具参考](../../docs/reference/tools.md)：带参数和示例的完整工具参考
- [技能参考](../../docs/guide/skills.md)：技能参考、工作流、使用场景与组合
- [语言支持](../../docs/reference/language-support.md)：语言覆盖矩阵
- [架构](../../docs/architecture/architecture.md)：系统设计与内部原理
- [推测式执行](../../docs/guide/speculative-execution.md)：模拟后再应用的工作流
- [LSP 一致性](../../docs/reference/lsp-conformance.md)：LSP 3.17 规范覆盖情况
- [Docker](../../DOCKER.md)：Docker 标签、compose 和卷缓存

### 贡献

- [CI 说明](../../docs/architecture/ci-notes.md)：CI 的特殊之处和测试框架细节
- [分发](../../docs/architecture/distribution.md)：安装渠道和发布流水线

## 开发

```bash
git clone https://github.com/blackwell-systems/agent-lsp.git
cd agent-lsp && go build ./...
go test ./...                   # unit tests
go test ./... -tags integration # integration tests (requires language servers)
```

## 库用法

`pkg/lsp`、`pkg/session` 和 `pkg/types` 包暴露了一套稳定的 Go API，可直接使用 agent-lsp 的 LSP 客户端，而无需运行 MCP 服务器。

```go
import "github.com/blackwell-systems/agent-lsp/pkg/lsp"

client := lsp.NewLSPClient("gopls", []string{})
client.Initialize(ctx, "/path/to/workspace")
defer client.Shutdown(ctx)

locs, err := client.GetDefinition(ctx, fileURI, lsp.Position{Line: 10, Character: 4})
```

完整的包 API 参见 [docs/architecture/architecture.md](../../docs/architecture/architecture.md)。

## 许可证

MIT
