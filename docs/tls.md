# HTTPS (TLS)

Bingle runs fine **without** TLS (plain HTTP, the default) and **with** it. HTTPS is
needed when players reach the server over the internet or a domain, and later for
voice chat (browsers only allow the microphone on HTTPS or `localhost`).

There are three ways to run it; pick one:

| Mode | Settings | Use it when |
|---|---|---|
| Plain HTTP | nothing | development, a trusted LAN |
| Automatic certificates | `BINGLE_TLS_DOMAIN=games.example.com` | the name leads to this machine and ports 80 and 443 reach it from the internet |
| Certificate files | `BINGLE_TLS_CERT` + `BINGLE_TLS_KEY` | you already have a certificate (a private CA, a proxy's, `lego`/`certbot` output) |

`BINGLE_TLS_DOMAIN` and the certificate files cannot both be set (bingle stops with an
error). Setting only one of `BINGLE_TLS_CERT`/`KEY` is a mistake that falls back to plain
HTTP with a warning in the log.

## Automatic certificates (Let's Encrypt)

```sh
BINGLE_TLS_DOMAIN=games.example.com \
BINGLE_TLS_EMAIL=you@example.com \
BINGLE_DB_PATH=/data/bingle.db \
./bin/bingle
```

* Bingle listens on `:443` (HTTPS) and `:80` (certificate challenges, and a redirect of
  everything else to HTTPS). `BINGLE_ADDR` and `BINGLE_HTTP_ADDR` change the ports; ports
  below 1024 need root, `setcap cap_net_bind_service`, or a lowered
  `net.ipv4.ip_unprivileged_port_start`.
* At start it fetches a certificate for each name and logs `certificate for … is ready`;
  if it cannot, it says why, keeps running and tries again when a browser connects.
  Renewal happens by itself, about 30 days before expiry.
* Certificates and the account key live in `BINGLE_TLS_CACHE` (default `certs` next to the
  database, so `/data/certs` in the container). **Keep that directory** across restarts, or
  every start asks for a new certificate and you will meet Let's Encrypt's rate limits.
* Only the configured names are served; a request for any other name (or a bare IP) gets
  no certificate.
* Using the service means accepting the certificate authority's terms of service;
  starting bingle with `BINGLE_TLS_DOMAIN` set counts as that acceptance.
* `BINGLE_PUBLIC_URL` defaults to `https://<first domain>`, so invite links use it. Set it
  yourself if players reach you on another port.
* `BINGLE_TLS_HSTS_SECONDS=86400` (say) adds `Strict-Transport-Security`. Leave it off
  until HTTPS has worked for a while: while it is in force, browsers refuse plain HTTP for
  the name.

### What has to be true

1. **The name leads to your home.** At your registrar (register.com for
   `example.com`) create a record for a host name such as `games.example.com`:
   * an **A** record with your public IPv4 address, or
   * a **CNAME** to the name your dynamic DNS client already keeps up to date
     (`yourname.example-ddns.net`). That keeps working when your IP changes, with nothing
     more to run. (A CNAME works for `games.example.com`, not for the bare
     `example.com`.)
   Check it with `dig +short games.example.com`, from a machine outside your network if
   you can. DNS changes can take a while to spread.
2. **Ports 80 and 443 reach the machine** that runs bingle: forward both on your router to
   it (TCP). Let's Encrypt needs one of them: 80 for the HTTP-01 challenge, 443 for
   TLS-ALPN-01; bingle answers both. If your provider blocks port 80, 443 alone is enough.
   If your ISP puts you behind carrier-grade NAT (your router's WAN address is not the
   address `curl ifconfig.me` shows, or is in 100.64.0.0/10), inbound forwarding cannot
   work, and certificates for this method cannot be issued.
3. **Players inside your own network** must reach the name too. Some routers do not loop a
   request for your public address back inside ("hairpin NAT"); if
   `https://games.example.com` works from a phone on mobile data but not from a laptop
   at home, give your router or local DNS an override that points the name at the
   machine's LAN address.

### Try it with the staging authority first

Let's Encrypt allows few failed attempts per hour on the real service. First run against
its staging directory, which issues certificates browsers do not trust but has generous
limits, and proves that DNS and the forwarding work:

```sh
BINGLE_TLS_DOMAIN=games.example.com \
BINGLE_TLS_ACME_URL=https://acme-staging-v02.api.letsencrypt.org/directory \
BINGLE_TLS_CACHE=/data/certs-staging \
./bin/bingle
```

Look for `certificate for games.example.com is ready` in the log, and open the site
(accept the browser's warning about the staging certificate). Then stop bingle, **remove
the staging cache directory**, and start again without `BINGLE_TLS_ACME_URL` for the real
certificate; the browser now shows a lock.

### Containers

The image listens on `8080` for plain HTTP. For HTTPS publish the standard host ports to
unprivileged ports inside:

```sh
podman run -d --name bingle \
  -e BINGLE_TLS_DOMAIN=games.example.com -e BINGLE_TLS_EMAIL=you@example.com \
  -e BINGLE_ADDR=:8443 -e BINGLE_HTTP_ADDR=:8081 \
  -p 443:8443 -p 80:8081 -v bingle-data:/data \
  bingle
```

(Rootless podman cannot publish ports below 1024 unless
`net.ipv4.ip_unprivileged_port_start=80` is set on the host, or you forward 80/443 to
higher host ports at the router.) The health check tries the plain port, HTTPS from files
and the redirect listener's `/healthz`, so it works in each mode.

## Certificate files

`BINGLE_TLS_CERT=/path/fullchain.pem BINGLE_TLS_KEY=/path/privkey.pem`: bingle serves HTTPS
from them on `BINGLE_ADDR` (default `127.0.0.1:8080`, so set it to something reachable when serving certificates) and does not renew or reload them; restart it
after renewing. This is also how to use a certificate from a DNS-01 client such as `lego`
(for names that cannot be reached from the internet) or from `certbot`.

## Running both ways

Nothing about TLS changes the database, the games or the invite codes, so switching
between plain HTTP and HTTPS is only a matter of the environment at start. Players'
remembered games are per origin in the browser (`http://…` and `https://…` are different
sites), so the list of active games they see is empty at first on the other address.
