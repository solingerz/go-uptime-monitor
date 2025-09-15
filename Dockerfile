FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git upx ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN GOMAXPROCS=1 GOGC=50 CGO_ENABLED=0 GOOS=linux go build \
    -p 1 \
    -a -trimpath \
    -ldflags="-linkmode=internal -s -w -extldflags '-static'" \
    -o uptime-monitor .

RUN upx --lzma --best uptime-monitor

FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/uptime-monitor .
COPY --from=builder /app/static ./static

USER 1000

EXPOSE 8080

CMD ["./uptime-monitor"]