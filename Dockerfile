# hgsd: 红果沙雕漫剧 service for the rex module
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/hgsd ./cmd/hgsd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata su-exec && adduser -D -u 1000 hgsd && mkdir /data && chown hgsd /data
ENV TZ=Asia/Shanghai
COPY --from=build /out/hgsd /usr/local/bin/hgsd
COPY docker-entrypoint.sh /usr/local/bin/
VOLUME /data
EXPOSE 8686
HEALTHCHECK --interval=60s --timeout=5s CMD wget -qO- http://127.0.0.1:8686/healthz || exit 1
ENTRYPOINT ["docker-entrypoint.sh"]
