FROM golang:1.27 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Byg hele pakken (main.go, weather.go osv.), ikke kun main.go, og værktøjet
# der opsummerer søge-loggen (docker exec whoknows-backend ./searchstats)
RUN go build -o backend . && go build -o searchstats ./cmd/searchstats

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/backend /app/searchstats ./
# Rodcertifikater, så backenden kan kalde Open-Meteo over HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/schema.sqlite .

EXPOSE 8080

CMD ["./backend"]