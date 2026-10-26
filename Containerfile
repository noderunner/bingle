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
VOLUME ["/data"]
# Plain HTTP uses 8080. For HTTPS see docs/tls.md: with BINGLE_TLS_DOMAIN set, run
# the container with BINGLE_ADDR=:8443 and BINGLE_HTTP_ADDR=:8081 and publish
# host ports 443 and 80 to them.
EXPOSE 8080 8081 8443
# Plain HTTP on 8080; HTTPS from files on 8080 (self-signed or private CA is fine
# for a health check); with automatic certificates the HTTP listener on 8081.
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS "http://127.0.0.1:8080/healthz" \
     || curl -fsSk "https://127.0.0.1:8080/healthz" \
     || curl -fsS "http://127.0.0.1:8081/healthz" \
     || exit 1
ENTRYPOINT ["/app/bingle"]
