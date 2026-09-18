# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /nimbus ./cmd/nimbus

FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ .
RUN npm run build

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget ffmpeg python3 py3-pip \
  && python3 -m pip install --no-cache-dir --break-system-packages --upgrade yt-dlp
WORKDIR /app
COPY --from=build /nimbus /app/nimbus
COPY --from=web /web/dist /app/web
COPY docs /app/docs
ENV DATA_DIR=/data
ENV WEB_DIR=/app/web
ENV PORT=8080
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/app/nimbus"]
