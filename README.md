# mnemonic

[![Build Status](https://github.com/jimschubert/mnemonic/actions/workflows/build.yml/badge.svg)](https://github.com/jimschubert/mnemonic/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jimschubert/mnemonic)](https://github.com/jimschubert/mnemonic/blob/main/go.mod)
[![License](https://img.shields.io/github/license/jimschubert/mnemonic?a=b&color=blue)](https://github.com/jimschubert/mnemonic/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/jimschubert/mnemonic)](https://goreportcard.com/report/github.com/jimschubert/mnemonic)
[![GitHub Release](https://img.shields.io/github/v/release/jimschubert/mnemonic)](https://github.com/jimschubert/mnemonic/releases/latest)

> Attention-based MCP memory controller for LLM coding agents.

## Installation

```shell
go install github.com/jimschubert/mnemonic/cmd/mnemonic@latest
```

Or, check the [releases page](https://github.com/jimschubert/mnemonic/releases) for binaries.

## Usage

Quick start:

```shell
mnemonic --help
```

## Configuration

### External config file

Reusable configuration can be defined in YAML files. These files are structured as commands with their respective flags.
Here is the order of precedence for configuration, from lowest to highest where higher values override lower values:

1. Any default values defined on the target command flag
2. `~/.mnemonic/config.yaml` — global/user config
3. `.mnemonic/config.yaml` — project-local config (relative to where mnemonic is invoked)
4. Environment variable
5. CLI flag

Example `.mnemonic/config.yaml`:

```yaml
server:
  global-dir: ~/.mnemonic
  local-dir: .mnemonic
  team:
    - /shared/team-data
  server-addr: localhost:20001

stdio:
  global-dir: ~/.mnemonic
  local-dir: .mnemonic
  team:
    - /shared/team-data
```

YAML keys must match the long flag names (see `--help`).

### Team directories

Pass one or more `--team` directories to load an additional shared scope per directory. 
Each team directory is registered as scope `team:<basename>`, so `/shared/acme` becomes scope `team:acme` and can be referenced
in your agent to access team-specific memory.

```shell
mnemonic server --team /shared/acme --team /shared/platform
```

### MCP server

Example MCP configuration (JetBrains IDEs):

```json
{
  "mcpServers": {
    "mnemonic": {
      "type": "http",
      "url": "http://localhost:20001/mcp"
    }
  }
}
```

Example memory instructions:

```markdown
## Memory
Before starting any task, call `mnemonic_query` with a description of
the work to retrieve relevant rules and lessons. Always query the
`avoidance` and `security` categories.

All available categories:
- avoidance   — mistakes, wrong approaches, things that don't work
- security    — security concerns or constraints
- architecture — design decisions and why they were made
- syntax      — code patterns that worked well
- domain      — project-specific knowledge

DO NOT create new categories without explicit instructions. If a new category is needed, add it to the list above and inform the user.

Set default scope to "project". Set default source to "agent:YYYY-MM-DD".

If the user prompt includes "remember this" or "add this to memory", always call `mnemonic_add` with the content.
Call `mnemonic_reinforce` with delta +0.1 for confirmed patterns, -0.2 for rejected ones.
```

## License

Apache 2.0 – see [LICENSE](LICENSE)
