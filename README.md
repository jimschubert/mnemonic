# mnemonic

[![Build Status](https://github.com/jimschubert/mnemonic/actions/workflows/build.yml/badge.svg)](https://github.com/jimschubert/mnemonic/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jimschubert/mnemonic)](https://github.com/jimschubert/mnemonic/blob/main/go.mod)
[![License](https://img.shields.io/github/license/jimschubert/mnemonic?a=b&color=blue)](https://github.com/jimschubert/mnemonic/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/jimschubert/mnemonic)](https://goreportcard.com/report/github.com/jimschubert/mnemonic)
[![GitHub Release](https://img.shields.io/github/v/release/jimschubert/mnemonic)](https://github.com/jimschubert/mnemonic/releases/latest)

> Attention-based MCP memory controller for LLM coding agents.

`mnemonic` turns project guidance, lessons learned, and architectural decisions into a shared MCP memory layer.
Instead of stuffing a growing pile of `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, and editor-specific instructions into
every prompt, agents can retrieve only the memories that matter for the task at hand.

It's built for people who use multiple agents, multiple IDEs, or multiple repositories and want one memory system that
stays searchable, versionable, and reusable.

The design mirrors the transformer attention mechanism:

| Transformer     | mnemonic                             |
|-----------------|--------------------------------------|
| Query (Q)       | The agent's current task             |
| Key (K)         | Entry tags and embeddings            |
| Value (V)       | Memory content injected into context |
| Attention heads | Memory categories                    |

## Motivation

`mnemonic` was inspired by a mix of:

* [Andrej Karpathy's LLM Wiki](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f)
* The [Attention Is All You Need](https://en.wikipedia.org/wiki/Attention_Is_All_You_Need) paper

### The problem

Agent memory is fragmented and expensive to maintain. Different tools expect different instruction files,
so shared guidance gets copy-pasted across repos and editors. Static files waste context window space
regardless of relevance, and when the context window fills up something gets dropped — often the detail
that mattered. High-value lessons learned during a session disappear into chat history instead of
becoming reusable knowledge.

### The solution

`mnemonic` exposes memory as MCP tools backed by *local YAML files* and optional *semantic search*.

Memory is organized into categories and scopes — global, project, or team — so agents query only
what's *relevant to the task at hand* rather than ingesting a monolithic instruction file every session.
Entries are version-controlled plain YAML, _scored by hit count and recency_, and _decay naturally over
time_ so high-signal memories stay visible without manual curation.

Unlike Karpathy's wiki (which is agent-controlled), `mnemonic` is built for collaboration between
agents and humans. Agents query for context and store lessons learned according to your system prompt,
but humans can also add entries, reinforce confirmed patterns, or demote approaches that didn't work.
Optional *semantic retrieval* via embeddings and a local HNSW index upgrades query quality significantly
once your memory store grows beyond a few dozen entries.

## Quick start

### Install

``` sh
go install github.com/jimschubert/mnemonic/cmd/mnemonic@latest
```

Or download a binary from the [releases page](https://github.com/jimschubert/mnemonic/releases).

### Configure your client

For clients that support stdio transports, use `mnemonic stdio`.

``` json
{
  "mcpServers": {
    "mnemonic": {
      "command": "mnemonic",
      "args": [
        "stdio"
      ]
    }
  }
}
```

If your client only supports HTTP, run `mnemonic server` and connect to
`http://localhost:20001/mcp`.

### Configure your agent

This is a good starting instruction block for most coding agents:

```markdown
## Memory

Before starting any task, call `mnemonic_query` with a description of the work.
Always query the `avoidance` and `security` categories first.

Available categories:

- avoidance — mistakes, failed approaches, things that do not work
- security — security constraints and risks
- architecture — design decisions and rationale
- syntax — patterns and code conventions that worked well
- domain — project-specific knowledge

Do not create new categories unless a human explicitly asks for one.
Default scope should be `project`.
Default source should be `agent:YYYY-MM-DD`.

If the user says "remember this" or "add this to memory", call `mnemonic_add`.
Use `mnemonic_reinforce` with `+0.1` for confirmed patterns and `-0.2` for rejected ones.
```

### Enable semantic search (optional)

`mnemonic` performs standard searches without embeddings, but semantic search is better. Configure an embedding endpoint
and build the local HNSW index to use semantic querying and deduplication.

``` yaml
# ~/.mnemonic/config.yaml
embeddings:
  endpoint: http://127.0.0.1:1234/v1/embeddings
  model: nomic-ai/nomic-embed-text-v1.5
```

``` sh
mnemonic embed
```

If embeddings are unavailable, `mnemonic` falls back to category and keyword search.

## MCP Tools

`mnemonic` exposes four MCP tools:

| Tool                  | Purpose                                                                          |
|-----------------------|----------------------------------------------------------------------------------|
| `mnemonic_query`      | Retrieve relevant memories for a task, optionally filtered by category and scope |
| `mnemonic_add`        | Store a new memory entry                                                         |
| `mnemonic_reinforce`  | Increase or decrease a memory's score                                            |
| `mnemonic_list_heads` | List available categories and entry counts                                       |

Typical flow:

1. Query `avoidance` and `security` first.
2. Query another category or a broader task description.
3. Use the returned memories while doing the work.
4. Store or reinforce anything worth keeping.

Example `mnemonic_query` input:

``` json
{
  "query": "update GitHub workflows for Go 1.26 and verify pwn-request safety",
  "category": "security",
  "top_k": 5,
  "scopes": [
    "project",
    "global"
  ]
}
```

Example `mnemonic_add` input:

``` json
{
  "content": "When an MCP stdio client receives `session not found`, invalidate the session and reconnect.",
  "category": "architecture",
  "tags": [
    "mcp",
    "stdio",
    "session-management"
  ],
  "scope": "project",
  "source": "agent:2026-04-19"
}
```

## How it works

### Runtime model

* `mnemonic stdio` is the default path for editor integrations.
    * It auto-starts the daemon if needed.
    * It proxies MCP calls over stdio to the daemon, which handles storage and embedding.
* `mnemonic server` starts the HTTP MCP server and daemon-backed storage directly.
    * You can start `mnemonic stdio` separately; it knows what to do.
* `mnemonic stop` asks the running daemon to shut down cleanly.
    * To avoid stale sessions and errors, any open `stdio` processes will detect the shutdown and exit.

### Storage model

Memory is stored as YAML on disk, grouped by **scope** and **category**.

| Scope         | Description                                 |
|---------------|---------------------------------------------|
| `global`      | User-wide memory shared across repositories |
| `project`     | Repository-local memory                     |
| `team:<name>` | 0..N shared team directories you opt into   |

Example of the default directory layout:

``` text
~/.mnemonic/
├── config.yaml
├── global/
│   ├── avoidance.yaml
│   ├── security.yaml
│   └── syntax.yaml
└── index.hnsw

.mnemonic/
└── project/
    ├── architecture.yaml
    └── domain.yaml
```

Each category file contains versioned entries such as:

``` yaml
version: 1
entries:
  - id: go-error-wrapping
    content:
      Wrap errors with context using fmt.Errorf("doing X: %w", err).
    tags: [ go, errors, style, fmt ]
    category: syntax
    scope: global
    score: 0.9
    hit_count: 12
    last_hit: 2026-04-08T00:00:00Z
    created: 2026-03-20T00:00:00Z
    source: manual
```

### Retrieval model

When embeddings are configured and indexed, `mnemonic_query` attempts semantic search first. If
embeddings are not configured or semantic lookup fails, it falls back to keyword and category-based
search.

Ranking is influenced by score, hit count, and recency of use.

That means important memories stay visible, but stale memories naturally decay over time.

## Commands

``` sh
mnemonic --help
```

| Command           | Description                                              |
|-------------------|----------------------------------------------------------|
| `mnemonic stdio`  | Serve MCP over stdio and auto-start the daemon if needed |
| `mnemonic server` | Start the HTTP MCP server and backing daemon             |
| `mnemonic embed`  | Fetch embeddings and build or refresh the HNSW index     |
| `mnemonic stop`   | Request shutdown of the running daemon                   |

### Useful examples

``` sh
mnemonic stdio
mnemonic server --server-addr localhost:9999
mnemonic server --team /shared/acme --team /shared/platform
mnemonic embed --force
mnemonic stop
```

## Configuration

Configuration is resolved in this order, highest precedence first:

1. CLI flags
2. Environment variables
3. `.mnemonic/config.yaml`
4. `~/.mnemonic/config.yaml`
5. Built-in defaults

Example global config:

``` yaml
log_level: info
server_addr: localhost:20001
socket_path: ~/.mnemonic/mnemonic.sock
client_timeout_sec: 5

logging:
  store: debug
  server: warn

embeddings:
  endpoint: http://127.0.0.1:1234/v1/embeddings
  model: nomic-ai/nomic-embed-text-v1.5

index:
  # NOTE: This must match the length of the vectors returned by the embedding endpoint
  # and is validated during `mnemonic embed` preflight.
  # For OpenAI's text-embedding-3-small, use 1536. For LM Studio's nomic-embed-text-v1.5, use 768.
  # A mismatch with an existing index requires a force rebuild with `mnemonic embed --force`.
  dimensions: 768
  # The number of bi-directional links created for each new entry.
  # A good default for OpenAI embeddings is 16.
  connections: 16
  # The level generation factor.
  # For 0.25, each layer is 1/4 the size of the previous layer.
  level_factor: 0.25
  # The maximum number of entries per layer. Higher values improve search accuracy at the expense of memory.
  # 20-50 is a reasonable default.
  ef_search: 50
```

Example project config:

``` yaml
log_level: debug
server_addr: localhost:9999
```

Key options:

| Option               | Purpose                                                       |
|----------------------|---------------------------------------------------------------|
| `log_level`          | Default log level                                             |
| `logging`            | Per-scope log levels, such as `store` or `server`             |
| `server_addr`        | HTTP MCP listen address                                       |
| `socket_path`        | Unix socket path used by the daemon                           |
| `client_timeout_sec` | Timeout for embedding and daemon HTTP clients                 |
| `embeddings.*`       | Embedding endpoint, model, auth token, and preflight behavior |
| `index.*`            | HNSW index parameters                                         |

For the full configuration surface, see [`internal/config/config.go`](./internal/config/config.go).

## Team scopes

Pass one or more `--team` directories to load additional shared scopes. Each team directory becomes
`team:<basename>`, so `/shared/acme` becomes `team:acme`.

``` sh
mnemonic server --team /shared/acme --team /shared/platform --server-addr localhost:9999
```

This makes it easy to layer memory like this:

* `global`: your personal reusable patterns
* `team:acme`: shared team conventions
* `project`: repo-specific context

## Embeddings and semantic search

Semantic search is optional, but it is one of the biggest quality-of-life upgrades once you have
more than a few dozen memories.

`mnemonic embed`:

* validates the embedding endpoint unless you disable preflight
* embeds stored entries
* builds or refreshes the HNSW index
* enables semantic retrieval for `mnemonic_query`

The default embedding settings are aimed at a local LM Studio-compatible endpoint, but any
compatible embeddings API should work if it returns vectors with the configured dimensions.

## License

Apache 2.0, see [LICENSE](LICENSE)
