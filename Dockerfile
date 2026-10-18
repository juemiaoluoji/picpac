# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/picpac ./cmd

FROM scratch
WORKDIR /app
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/picpac /app/picpac
COPY category.json /app/category.json

ENV PICPAC_CONFIG=/app/config/config.json \
    PICPAC_CATEGORY_CONFIG=/app/category.json
USER 10001:10001
EXPOSE 9090
ENTRYPOINT ["/app/picpac"]
