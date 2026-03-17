FROM golang:1.25-alpine AS build
LABEL org.opencontainers.image.source="https://github.com/omnivore-app/omnivore"

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o omnivore .

# ─── Runtime image ────────────────────────────────────────────────────────────
FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/omnivore-app/omnivore"

RUN apk add --no-cache ca-certificates && rm -rf /var/cache/apk/*

WORKDIR /app

ENV PORT=3002

COPY --from=build /app/omnivore .

EXPOSE 3002

CMD ["./omnivore", "server", "queue-processor"]
