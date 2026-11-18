FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gymbag ./cmd/gymbag

FROM alpine:3.21

RUN apk add --no-cache ca-certificates libcap \
    && adduser -D -u 10001 -h /app gymbag \
    && mkdir -p /app/certs \
    && chown -R gymbag:gymbag /app

COPY --from=builder /out/gymbag /usr/local/bin/gymbag
RUN setcap 'cap_net_bind_service=+ep' /usr/local/bin/gymbag

WORKDIR /app
COPY --chown=gymbag:gymbag Gymbagfile /app/Gymbagfile

ENV GYMBAG_BIND=0.0.0.0 \
    GYMBAG_CONFIG=/app/Gymbagfile

USER gymbag
EXPOSE 80 443

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["nc", "-z", "127.0.0.1", "80"]

ENTRYPOINT ["gymbag"]
CMD ["run", "--config", "/app/Gymbagfile"]
