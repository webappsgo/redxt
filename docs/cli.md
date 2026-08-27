# CLI Reference

`redxt-cli` is the required companion client (CLI/TUI) for redxt. It
talks to a running `redxt` server over HTTP.

## Global Flags

Available at every command level, both as `--flag value` and
`--flag=value`:

- `--help` / `-h` — identical output to the bare `help` command
- `--version` / `-v`
- `--debug` — debug output
- `--color` (`auto` by default; `NO_COLOR` and `TERM=dumb` always
  disable color)
- `--server` — target redxt server URL
- `--token` — API token for authentication
- `--token-file` — read the API token from a file instead
- `--user` — target user or org (`@user`, `+org`)
- `--config` — config profile name (default `cli.yml`)
- `--lang` — language for output
- `--shell` — shell integration: `completions`, `init`, or `help`

## Environment Variables

| Variable | Effect |
|---|---|
| `REDXT_TOKEN` | API token for `redxt-cli`, used when `--token`/`--token-file` are unset |
| `REDXT_AGENT_TOKEN` | Enrollment token for `redxt-agent` |
| `NO_COLOR` | Disables color output on both binaries, regardless of `--color` |
| `TERM=dumb` | Disables color output on both binaries |

Tokens resolve in priority order: `--token` flag > token file >
environment variable > config file.

## Health

```bash
redxt-cli --server http://localhost:64580 health
```

Calls the unauthenticated `GET /server/healthz` endpoint and reports
the server's health state. Running `redxt-cli` with no command starts
the interactive TUI instead.

## Shell Completions

The shell is an optional positional argument; it is auto-detected when
omitted.

```bash
redxt-cli --shell completions bash
redxt-cli --shell completions zsh
redxt-cli --shell init fish
```

## Binary Renaming

`redxt-cli` reads `argv[0]` and reflects the actual invoked name in its
`--help` output, so it can be renamed or symlinked freely.

## redxt-agent

`redxt-agent` runs on remote/managed nodes and shares the same global
flag conventions (`--help`, `--version`, `--debug`, `--color`,
`--lang`, `--shell`, `NO_COLOR`/`TERM=dumb` support). It additionally
accepts `--config`, `--data`, `--log`, `--server`, `--token`, `--mode`,
and `--status`.

```bash
redxt-agent --server http://localhost:64580 --status
```

`--status` reports agent health and exits `0` when healthy, `1` when
unhealthy.
