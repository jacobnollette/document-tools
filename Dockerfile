# Stage 1: build the web app
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: build the Go server
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server

# Stage 3: runtime with the document toolchain — tesseract for OCR, poppler
# for PDF text/preview extraction, pandoc + typst for markdown → PDF.
FROM alpine:3.20
RUN apk add --no-cache \
    tesseract-ocr tesseract-ocr-data-eng \
    poppler-utils \
    pandoc-cli typst \
    ca-certificates \
    && adduser -D app \
    && mkdir -p /data && chown app /data

COPY --from=build /out/server /usr/local/bin/server
COPY --from=web /src/web/dist /srv/web

USER app
ENV LISTEN_ADDR=:8080 \
    DATA_DIR=/data \
    WEB_DIST=/srv/web
EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/server"]
