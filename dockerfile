# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.5

FROM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS
ARG TARGETARCH
ARG TEMPL_VERSION=v0.3.1020

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

RUN if [ -n "${TEMPL_VERSION}" ]; then \
        go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION}; \
    else \
        go install github.com/a-h/templ/cmd/templ@latest; \
    fi

COPY . .

RUN templ generate

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/kinops \
      ./cmd/kinops

RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /etc/ssl/certs/ca-certificates.crt \
    /etc/ssl/certs/ca-certificates.crt

COPY --from=build --chown=nonroot:nonroot /out/kinops /app/kinops
COPY --chown=nonroot:nonroot web/static /app/web/static
COPY --from=build --chown=nonroot:nonroot /out/data /data

VOLUME ["/data"]

EXPOSE 8081

ENV KINOPS_LISTEN_ADDRESS=:8081
ENV KINOPS_DATABASE_PATH=/data/kinops.db
ENV KINOPS_TIMEZONE=America/New_York

USER nonroot:nonroot

ENTRYPOINT ["/app/kinops"]
