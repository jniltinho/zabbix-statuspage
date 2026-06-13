FROM golang:alpine AS builder

RUN apk add --no-cache curl && \
    curl -fsSL https://github.com/tailwindlabs/tailwindcss/releases/download/v4.2.0/tailwindcss-linux-x64 \
    -o /usr/local/bin/tailwindcss && \
    chmod +x /usr/local/bin/tailwindcss

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN tailwindcss -i web/tailwindcss/input.css -o web/static/css/style.css --minify

ARG VERSION=dev
ARG BUILD_DATE=Unknown
ARG GIT_COMMIT=Unknown

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w \
      -X 'zabbix-statuspage/cmd.Version=${VERSION}' \
      -X 'zabbix-statuspage/cmd.BuildDate=${BUILD_DATE}' \
      -X 'zabbix-statuspage/cmd.GitCommit=${GIT_COMMIT}'" \
    -o zabbix-statuspage .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/zabbix-statuspage .

EXPOSE 3000

ENTRYPOINT ["./zabbix-statuspage", "serve"]
