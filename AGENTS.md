# AGENTS.md

Dynamic-DNS updater for Cloudflare. The host crontab runs the installed
binary (`~/.local/bin/cloudflare`) every 10 minutes; nothing else uses it.

## Compatibility contract

The crontab entry must keep working unchanged. Do not rename or remove
options, or move `~/.dns.log`, without being asked to.

Exit code 0 means everything asked for was done; anything else exits 1.

## Decided: do not change in passing

- Records are written one at a time, not through the batch endpoint, so that
  one refused record does not block the others.
- `~/.dns.log` is not rotated by this tool.
- Authentication uses the Global API Key (`X-Auth-Email` / `X-Auth-Key`).
- Build-time values (TTL, names exempt from the proxy) live in `config.go`,
  not in options or a config file.

## Working rules

- Tests must not touch the network; use `newFakeAPI` in cloudflare_test.go.
- `make install` replaces the binary cron is using. Rehearse with
  `make install PREFIX=<scratch dir>` first.
