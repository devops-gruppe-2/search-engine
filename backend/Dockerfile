FROM golang:1.27 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o backend main.go

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/backend .
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/schema.sqlite .

EXPOSE 8080

CMD ["./backend"]