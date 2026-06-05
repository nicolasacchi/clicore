# clicore

Shared core for the 1000farmacie custom CLI fleet (`ddx`, `otx`, `gx`, `stx`,
`chx`, `kv`, `mbx`, `sgx`, `gumlet`, `ga-cli`, `merchant-cli`). It encodes the
fleet's correct conventions **once** so each tool stops hand-copying — and
silently diverging on — the same four-layer scaffold.

```
go get github.com/nicolasacchi/clicore@latest
```

## Packages

| Package | What it gives you |
|---------|-------------------|
| `httpclient` | Retry loop (max 3, exp backoff + jitter, `Retry-After`) with two safety fixes baked in: **never retry POST/PATCH on a network error** (a mid-flight failure may already have committed the write) and **never retry a permanent error** (TLS/x509, DNS NXDOMAIN, ctx cancel). Clean 429/5xx is retried for any method. Pluggable `Authorizer` (bearer/basic/header + refresh-on-401). |
| `config` | TOML multi-project store with **atomic `Save()`** (temp file + `os.Rename`, so an interrupted encode never corrupts a config that holds credentials), `0700`/`0600` perms, `FirstNonEmpty` (flag>env>file), `MaskSecret`. |
| `output` | `AgentMode` (`CLAUDECODE`), `IsJSON` (TTY/`--json`/`--jq`), `ResolveLimit` + `CapRows` + `TruncatedMarker` so every tool gets the agent row-cap + `{"truncated":true,"shown":N,"total":M}` marker for free. |
| `cierrors` | Canonical `APIError{Status,Kind,Detail,Hint}` + the **single fleet exit-code table** (`ExitCode()`), `KindForStatus`. |
| `confirm` | Write-safety gate: `Require`/`Gate` refuse destructive verbs unless `--yes`, returning a `write_locked` `cierrors.APIError` (exit 6); `--dry-run` previews. No cobra dependency — pass primitives + `cmd.OutOrStdout()`. |
| `redact` | Masks credential keys + email/phone PII in `--verbose` request/response bodies and URLs (`Body`/`URL`/`Token`). |

## Exit-code table (canonical, via `cierrors.APIError.ExitCode()`)

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | generic / network |
| 2 | auth_failed / forbidden (401, 403) |
| 3 | validation (400, bad args) |
| 4 | not_found (404) |
| 5 | rate_limited (429) |
| 6 | write_locked / deprecated_endpoint (refused mutation) |
| 7 | async_timeout |

## Migrating a tool

See [`MIGRATION.md`](MIGRATION.md). One PR per tool, lowest-risk first, each
gated by `go test ./...` staying green.

Origin: the 2026-05-29 CLI fleet review (`platform_context/ideas/cli-fleet-review/`),
fix `cli-fleet-shared-core-go-module`.
