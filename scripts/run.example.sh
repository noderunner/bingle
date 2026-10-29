#!/bin/bash
# An example of starting bingle with HTTPS, email notifications and a TURN relay.
# Copy it to run_bingle.sh (which git ignores), fill in your own values, and run it
# from the repository root after `make build`. Every setting is optional: plain
# `./bin/bingle` serves http://localhost:8080 with nothing else set up.
# See docs/build.md for the full list of variables.
set -euo pipefail

# Voice chat through a TURN relay (docs/voice.md): this file would set
# BINGLE_RTC_TURN_URLS and BINGLE_RTC_TURN_SECRET. Leave it out for STUN only.
# source ~/.config/bingle/turn.env

export BINGLE_TLS_DOMAIN=bingle.example.com          # automatic Let's Encrypt certificates (docs/tls.md)
export BINGLE_TLS_EMAIL=you@example.com
export BINGLE_PUBLIC_URL=https://bingle.example.com   # the address in invite links and mail
export BINGLE_HTTP_ADDR=:8080                         # the HTTP listener for the certificate challenge
export BINGLE_ADDR=:8443                              # the HTTPS listener
export BINGLE_SMTP_HOST=smtp.example.com              # email notifications (docs/email.md)
export BINGLE_SMTP_PASSWORD_FILE="$HOME/.config/bingle/smtp_password"
export BINGLE_LOG_LEVEL=info                          # debug for a line per request (docs/observability.md)
export BINGLE_LOG_FORMAT=text                         # json for a log collector
export BINGLE_METRICS_ADDR=127.0.0.1:9095             # curl it for Prometheus metrics; off if empty
# A token is required only if the line above leaves the loopback interface:
# export BINGLE_METRICS_TOKEN="$(cat ~/.config/bingle/metrics_token)"
export BINGLE_SLOW_REQUEST=250ms                      # write slow requests down (docs/observability.md)
export BINGLE_METRICS_SAMPLE=5m                       # metric history, and the page at /metrics/health
# BINGLE_PARANOID and BINGLE_DEBUG_SECRETS are for development only; leave them off here.
# export BINGLE_DEBUG_DIR=/var/lib/bingle/debug    # repro bundles when something impossible happens

exec ./bin/bingle
