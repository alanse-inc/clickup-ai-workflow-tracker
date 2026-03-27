FROM golang:1.26.1 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /server /usr/local/bin/server
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
