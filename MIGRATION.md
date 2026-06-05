# Migrating a CLI onto clicore

Apply per tool, **one PR each, lowest-risk first**. Each PR keeps the tool's own
`go test ./...` green and adds the smoke checks below. clicore is published at
`github.com/nicolasacchi/clicore` (private) — consumers `require` it; while the
fleet is monorepo-vendored you may instead add a `replace` to a local checkout.

## Order

1. **otx, mbx** — already shaped like the target; pure refactor that proves the
   seams. otx gains the atomic-write fix for free.
2. **stx, chx** — payoff PRs: delete the local `shouldRetryNetwork`
   (`return err != nil`) and adopt `clicore/httpclient` → closes the
   retry-on-non-idempotent-POST and retry-on-permanent-error bugs at once.
3. **ddx, kv, sgx, gumlet, ga-cli, merchant-cli** — inherit CLAUDECODE caps +
   structured error kind + canonical exit codes by construction.
4. **gx** — keep its GraphQL specifics; adopt `clicore/{config,output,cierrors}`.

Already shipped fleet-wide (do **not** re-copy — replace the per-tool copies
with the clicore packages during migration): the `redact` and `confirm`
packages, and `write_locked`→exit 6.

## Per-tool checklist

- [ ] `require github.com/nicolasacchi/clicore <version>` in `go.mod`; normalize the `go 1.26.x` directive (drift was 1.26.1 vs 1.26.2).
- [ ] Replace `internal/client` retry loop with `httpclient.Client` + an `Authorizer` (bearer/header). Delete any `shouldRetryNetwork`.
- [ ] Replace `internal/config` `Save` with `config.Save` (atomic). Keep tool-specific project fields by embedding or extending `config.Project`.
- [ ] Route list output via `output.ResolveLimit` + `output.CapRows` + `output.TruncatedMarker`.
- [ ] Map errors to `cierrors.APIError`; `main.go` exits via `.ExitCode()`.
- [ ] Replace the local `internal/redact` and `internal/commands/confirm.go` with `clicore/redact` and `clicore/confirm`.
- [ ] Drop the per-tool CI Go pin; the shared `ci.yml` uses `go-version-file: go.mod`.

## Smoke checks (per tool, after migrating)

```sh
# agent row-cap marker present when a list exceeds the cap
CLAUDECODE=1 <tool> <list-cmd> --json | jq '.truncated'

# atomic write: kill mid config-add leaves the prior config.toml intact
<tool> config add tmp --api-key x & sleep 0.05; kill -9 $!; <tool> config list
```

## Fleet-wide acceptance (run from the monorepo root once all tools migrate)

```sh
grep -rn 'return err != nil' cli_tools/*/internal/client/   # 0 shouldRetryNetwork hits
grep -rn 'O_TRUNC'           cli_tools/*/internal/config/    # 0 hits
```
