FROM golang:1.25-alpine AS build
LABEL org.opencontainers.image.source="https://github.com/omnivore-app/omnivore"

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o omnivore .

FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/omnivore-app/omnivore"

RUN echo "@edge https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories \
 && echo "@edge https://dl-cdn.alpinelinux.org/alpine/edge/main" >> /etc/apk/repositories \
 && echo "@edge https://dl-cdn.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories \
 && apk -U upgrade \
 && apk add --no-cache \
      chromium@edge \
      freetype@edge \
      ttf-freefont@edge \
      nss@edge \
      libstdc++@edge \
      sqlite-libs@edge \
      postgresql-client \
      ca-certificates@edge \
 && rm -rf /var/cache/apk/*

WORKDIR /app

ENV CHROMIUM_PATH=/usr/bin/chromium
ENV LAUNCH_HEADLESS=true
ENV PORT=8080

RUN wget -q -O /etc/hosts.blocklist https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts

COPY --from=build /app/omnivore .
COPY --from=build /app/migrations ./migrations
COPY --from=build /app/bootstrap-db.sh ./bootstrap-db.sh

RUN printf '#!/bin/sh\ncat /etc/hosts.blocklist >> /etc/hosts\nexec "$@"\n' > /entrypoint.sh \
 && chmod +x /entrypoint.sh /app/bootstrap-db.sh

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./omnivore", "server", "content-fetcher"]
