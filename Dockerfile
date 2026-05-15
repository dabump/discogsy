# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/discogsy .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /data/posters /app/internal/web \
    && chown -R app:app /data /app

WORKDIR /app

COPY --from=build /out/discogsy /usr/local/bin/discogsy
COPY internal/web/index.html /app/internal/web/index.html

USER app

ENV PORT=8082 \
    COLLECTION_PATH=/data/discogs_collection.json \
    POSTER_DIR=/data/posters

EXPOSE 8082

CMD ["discogsy"]
