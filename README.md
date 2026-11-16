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
  --force
  --list_zones
  --list_dns
  --update
  -h, --help          Show this message and exit
```

`--email` and `--key` (the Cloudflare Global API Key) are always required,
together with exactly one of `--list_zones`, `--list_dns` and `--update`.

**Update (or insert) DNS records**

```bash
cloudflare --email <email> --key <key> --update --zone example.domain --dns 'music,,videos,photos'
```

This points `music.example.domain`, `example.domain`, `videos.example.domain`
and `photos.example.domain` at the current public IP as `A` records. An empty
name stands for the zone itself.

- A record that already holds the right address is left alone. `--force`
  writes every record regardless.
- A missing record is created.
- With `--proxy`, the records are served through Cloudflare's proxy, except
  for the zone itself and the names listed in `config.go`.
- The TTL is set in `config.go` (120 seconds).

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

`--update` writes log lines to the terminal and appends the same lines to
`~/.dns.log`: one line per record it wrote, then one summary line.

```
2026-11-16 20:00:37.261 INFO  music.example.domain - {"result":{...},"success":true,...}
2026-11-16 20:00:37.305 INFO  example.domain - 4 names at 203.0.113.9: 1 updated, 3 unchanged, 0 failed
```

When something goes wrong:

```
2026-11-16 20:00:37.261 WARN  music.example.domain - Update failed: {"success":false,"errors":[...]}
2026-11-16 20:00:37.305 ERROR example.domain - 4 names at 203.0.113.9: 0 updated, 3 unchanged, 1 failed
```

`INFO` and `WARN` lines go to stdout, `ERROR` lines to stderr. The log file is
never rotated.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Everything asked for was done. |
| 1 | Invalid options, the public IP could not be determined, the Cloudflare API could not be reached or refused a request, or at least one record could not be updated. |
