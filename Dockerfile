FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o dbinsights-exporter ./cmd

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/dbinsights-exporter /bin/dbinsights-exporter
EXPOSE 8081
USER nobody
ENTRYPOINT ["/bin/dbinsights-exporter"]
CMD ["-config", "/etc/dbinsights/config.yml"]
