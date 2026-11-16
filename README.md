# cloudflare-cli

A small command-line tool that keeps Cloudflare DNS records pointed at this
machine's public IP address (dynamic DNS). It is a single static binary with
no runtime dependencies.

## Install

```bash
make install    # builds and copies the binary to ~/.local/bin/cloudflare
```

Building needs Go. `make test` runs the tests, `make uninstall` removes the
binary.

## Usage

```
Usage: cloudflare [<options>]

Options:
  --email=<email>
  --key=<token>
  --zone=<zone>
  --dns=<dns(,dns)*>
  --proxy
  --list_zones
  --list_dns
  --update
  -h, --help          Show this message and exit
```

`--email` and `--key` (the Cloudflare Global API Key) are always required.

**Update (or insert) DNS records**

```bash
cloudflare --email <email> --key <key> --update --zone example.domain --dns 'music,,videos,photos'
```

This points `music.example.domain`, `example.domain`, `videos.example.domain`
and `photos.example.domain` at the current public IP as `A` records with a
TTL of 120 seconds. An empty name stands for the zone itself.

With `--proxy`, the records are served through Cloudflare's proxy, except for
the zone itself and the `hub` subdomain.

**List zones**

```bash
cloudflare --email <email> --key <key> --list_zones

# Output:
# ──────────────
# example.domain
# ──────────────
```

**List DNS records**

```bash
cloudflare --email <email> --key <key> --list_dns --zone example.domain

# Output:
#  music.example.domain
#        example.domain
# videos.example.domain
# photos.example.domain
```

## Log

Every update writes one line per record to stdout and appends the same line
to `~/.dns.log`:

```
2026-11-16 20:00:37.261 INFO  music.example.domain - {"result":{...},"success":true,...}
2026-11-16 20:00:38.102 WARN  videos.example.domain - Update failed: zone example.domain not found
```

The log file is never rotated.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Done. A record that could not be updated is logged as `WARN` and does not change the exit code. |
| 1 | Invalid options, or the Cloudflare API or the IP lookup service could not be reached. |
