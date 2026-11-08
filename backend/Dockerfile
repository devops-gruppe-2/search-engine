FROM golang:1.27 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Byg hele pakken (main.go, weather.go osv.), ikke kun main.go
RUN go build -o backend .

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/backend .
# Rodcertifikater, så backenden kan kalde Open-Meteo over HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/schema.sqlite .

EXPOSE 8080

CMD ["./backend"]