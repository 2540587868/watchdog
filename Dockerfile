FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates \
    && go env -w GOPROXY=https://goproxy.cn,direct

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=none
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME} -s -w" \
    -o /bin/watchdog ./cmd/watchdog/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -g '' watchdog

COPY --from=builder /bin/watchdog /usr/local/bin/watchdog

RUN mkdir -p /data && chown watchdog /data

USER watchdog

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

VOLUME ["/data"]

ENTRYPOINT ["watchdog"]
CMD ["-config", "/etc/watchdog/config.yaml"]
