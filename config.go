package main

// Values fixed at build time. To change one, edit it here and reinstall.

// recordTTL is the TTL, in seconds, of every record this tool writes.
const recordTTL = 120

// unproxiedSubdomains are never served through Cloudflare's proxy, even with
// --proxy. The zone itself is never proxied either.
var unproxiedSubdomains = []string{"hub"}
