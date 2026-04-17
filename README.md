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

### Config file precedence

Configuration is resolved in order of precedence (highest wins):

1. CLI flags (e.g., `--server-addr localhost:9999`)
2. Environment variables (e.g., `MNEMONIC_SERVER_ADDR=localhost:9999`)
3. `.mnemonic/config.yaml` — project-local config (relative to where mnemonic is invoked)
4. `~/.mnemonic/config.yaml` — global/user config
5. Struct defaults defined in code

### Example config file

Global config (`~/.mnemonic/config.yaml`):

```yaml
log_level: info
server_addr: localhost:20001
socket_path: ~/.mnemonic/mnemonic.sock
client_timeout_sec: 5

# optional scoped logging levels
logging:
  store: debug
  server: warn
```

Project config (`.mnemonic/config.yaml`):

```yaml
log_level: debug
server_addr: localhost:9999
```

For available config options, see [Config struct](./internal/config/config.go).

### Team directories

Pass one or more `--team` directories to load an additional shared scope per directory. 
Each team directory is registered as scope `team:<basename>`, so `/shared/acme` becomes scope `team:acme` and can be referenced
in your agent to access team-specific memory.

For example:

```shell
mnemonic server --team /shared/acme --team /shared/platform --server-addr localhost:9999
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
