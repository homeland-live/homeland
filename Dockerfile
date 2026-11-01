# syntax=docker/dockerfile:1
ARG GO_VERSION=1.27
# keep in sync with ci.yml and docs.yml
ARG NODE_VERSION=24.22

# CGO needs a C toolchain for the target, so this stage runs on each target
# platform (under QEMU when it differs from the builder's).
FROM golang:${GO_VERSION}-alpine AS server-builder
ARG GITHUB_SHA
ARG GITHUB_REF
RUN apk add --no-cache ca-certificates tzdata make gcc musl-dev
WORKDIR /src
COPY go.mod go.sum Makefile ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build \
    make build STATIC=1
# scratch has no /tmp, where SQLite writes its temporary files. Copying the
# parent, not tmp itself, keeps tmp's sticky 1777 mode.
RUN mkdir -p /rootfs/tmp && chmod 1777 /rootfs/tmp

# The UI is platform-independent, so it is built once, natively.
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS ui-builder
WORKDIR /src
COPY ui/package.json ui/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --progress=false --no-audit --loglevel=error
COPY ui .
RUN npm run build

FROM scratch
WORKDIR /app
COPY --from=ui-builder /src/dist /app/ui/dist
COPY --from=server-builder /src/bin/server /app/server
COPY --from=server-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=server-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=server-builder /rootfs/ /
USER 1000:1000
# HL_HTTP_PORT's default.
EXPOSE 8080
CMD ["/app/server"]
