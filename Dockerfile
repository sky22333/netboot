FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w" -o /pxe ./cmd/pxe

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /pxe /usr/local/bin/pxe
VOLUME ["/data"]
EXPOSE 8088/tcp 80/tcp 67/udp 69/udp 4011/udp 6969/tcp
ENTRYPOINT ["pxe"]
CMD ["--data-dir", "/data", "--host", "0.0.0.0", "--port", "8088", "--no-browser"]
