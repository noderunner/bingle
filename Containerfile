FROM docker.io/library/golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/bingle .

FROM docker.io/library/alpine:3.22
RUN apk add --no-cache curl
WORKDIR /app
COPY --from=builder /out/bingle .
ENV BINGLE_ADDR=:8080
ENV BINGLE_DB_PATH=/data/bingle.db
# A container's port is published to a network by definition, so the image must
# satisfy the secure-connection rule (ADR 0065) rather than refuse to start.
# It generates and caches its own certificate, which lives in the volume and so
# survives a restart. Replace it for a real deployment:
#
#   automatic certificates:  -e BINGLE_TLS_SELFSIGNED=0 -e BINGLE_TLS_DOMAIN=games.example.com \
#                            -e BINGLE_ADDR=:8443 -e BINGLE_HTTP_ADDR=:8081  (publish 443 and 80)
#   your own certificate:    -e BINGLE_TLS_SELFSIGNED=0 -e BINGLE_TLS_CERT=… -e BINGLE_TLS_KEY=…
#   behind a proxy:          -e BINGLE_TLS_SELFSIGNED=0 -e BINGLE_TRUST_PROXY=1
ENV BINGLE_TLS_SELFSIGNED=1
VOLUME ["/data"]
EXPOSE 8080 8081 8443
# HTTPS on 8080 by default (the generated certificate, hence -k); plain HTTP if
# the image is reconfigured for a proxy; 8081 is the ACME challenge listener.
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsSk "https://127.0.0.1:8080/healthz" \
     || curl -fsS "http://127.0.0.1:8080/healthz" \
     || curl -fsS "http://127.0.0.1:8081/healthz" \
     || exit 1
ENTRYPOINT ["/app/bingle"]
