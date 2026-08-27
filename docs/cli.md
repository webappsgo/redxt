# CLI Reference

`redxt-cli` is the required companion client (CLI/TUI) for redxt. It
talks to a running `redxt` server over HTTP.

## Global Flags

Available at every command level, both as `--flag value` and
`--flag=value`:

- `--help` / `-h` — identical output to the bare `help` command
- `--version` / `-v`
- `--debug`
- `--color` (`auto` by default; `NO_COLOR` and `TERM=dumb` always
  disable color)
- `--server` — target redxt server URL

## Status

```bash
redxt-cli --server http://localhost:64580 status
```

Calls the unauthenticated `GET /server/healthz` endpoint and reports
the server's health state.

## Shell Completions

```bash
redxt-cli completions --shell bash
redxt-cli completions --shell zsh
redxt-cli completions --shell fish
```

## Binary Renaming

`redxt-cli` reads `argv[0]` and reflects the actual invoked name in its
`--help` output, so it can be renamed or symlinked freely.

## redxt-agent

`redxt-agent` runs on remote/managed nodes and shares the same global
flag conventions (`--help`, `--version`, `--debug`, `--color`,
`NO_COLOR`/`TERM=dumb` support).
