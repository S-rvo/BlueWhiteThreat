FROM golang:1.24-alpine AS build
WORKDIR /src
COPY pkg/csamfilter ./pkg/csamfilter
COPY apps/telegram ./apps/telegram
WORKDIR /src/apps/telegram
RUN go build -trimpath -o /telegram ./cmd/telegram

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S telegram && adduser -S -G telegram telegram && mkdir /data && chown telegram:telegram /data
COPY --from=build /telegram /usr/local/bin/telegram
USER telegram
WORKDIR /data
ENTRYPOINT ["telegram", "-registry", "/data/candidates.json"]
