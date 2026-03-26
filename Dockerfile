FROM golang:1.26.1 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM debian:bookworm-slim

# clickup へのリクエストに ca-certificates が必要
RUN apt update \
    && apt install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -s /usr/sbin/nologin nonroot

COPY --from=builder /server /usr/local/bin/server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
