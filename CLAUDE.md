# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build a static binary (./cloudflare)
make build

# Run vet and tests
make test

# Run a single test
go test -run TestUpdateWithProxyProxiesAllButZoneAndHub ./...

# Install to ~/.local/bin/cloudflare (atomic replace)
make install

# Try an install without touching the real one
make install PREFIX=/some/scratch/dir
```

## What this is

A dynamic-DNS updater for Cloudflare, run from the host crontab every 10
minutes. It looks up this machine's public IP and points a list of `A` records
at it. It was ported from Kotlin; the crontab entry was not changed, so the
option names, exit codes and help text are a compatibility contract.

## Architecture

One `package main`, mirroring the original Kotlin files:

- **cli.go** – option parsing (pflag) and dispatch to one of three modes:
  `--list_zones`, `--list_dns`, `--update`. First matching mode wins; with no
  mode it does nothing and exits 0. Holds the proxy rule (`forWeb`): with
  `--proxy`, everything is proxied except the zone itself and `hub.<zone>`.
- **cloudflare.go** – Cloudflare API v4 client. `updateRecord` looks up the
  zone id and the record id on every call, then PUTs (record exists) or POSTs.
- **param.go** – request body of a DNS record.
- **table.go** – single-column right-aligned table for the list modes; a port
  of the subset of the `cjk-table` library the Kotlin version used.
- **logger.go** – writes each line to stdout and appends it to `~/.dns.log`.

## Behaviour that looks odd but is intentional

These were carried over from the Kotlin version to keep the port
behaviour-preserving. Change them deliberately, not as a drive-by:

- A failed update is logged as `WARN` and the exit code stays 0.
- Zones and records are fetched again for every name (no caching, no
  pagination).
- The public IP comes from `http://ipinfo.io/ip` and is not validated.
- Authentication uses the Global API Key (`X-Auth-Email` / `X-Auth-Key`).
- `~/.dns.log` is never rotated.

## Tests

Tests never touch the network. `newFakeAPI` (cloudflare_test.go) is a local
stand-in for the Cloudflare API, and `runCli` (cli_test.go) runs the whole CLI
against it. Expected help and error texts were captured from the Kotlin
binary; expected tables from the original `cjk-table` library.
