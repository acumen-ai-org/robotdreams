# Container image for the `dream` CLI — primarily to run the control plane
# (`dream server init`) on Kubernetes or any container host, though the
# image carries the whole CLI so a worker can use it too.
#
#   docker build -t dream .
#   docker run --rm -p 7420:7420 -v dream-data:/data dream
#
# The default command binds all interfaces with --insecure: plaintext
# HTTP inside the container network, on the assumption that a
# TLS-terminating ingress or proxy sits in front. To serve HTTPS from the
# container itself, mount the PEM files and override the command:
#
#   docker run ... -v /path/to/tls:/tls:ro dream \
#     server init --data-dir /data --addr :7420 \
#     --tls-cert /tls/cert.pem --tls-key /tls/key.pem --reports /reports
#
# Build notes:
#   - Three stages. `ui` builds Mission Control (internal/dashboard/ui,
#     Vite) into internal/dashboard/web/dist/; `build` compiles Go with that
#     output embedded (the repository only commits a placeholder page —
#     see internal/dashboard/embed.go); the final stage is runtime only.
#   - The ui stage copies the workspace manifests first so `npm ci` is
#     cached until a lockfile changes, then the source.
#   - CGO_ENABLED=0 + modernc.org/sqlite (pure Go) gives a static binary;
#     the runtime image is alpine rather than scratch only so operators can
#     `kubectl exec ... cat <data-dir>/enrollment.token` (the enrollment
#     token is never printed to logs by design).
#   - The report definition library ships at /reports so a deployment can
#     start with `--reports /reports` without mounting anything.
#   - Runs as a non-root user. /data exists and is owned by that user, so
#     the image runs without a mount; mount a volume there to keep state.
#   - `dream server init` refuses a non-loopback --addr without
#     --tls-cert/--tls-key unless --insecure is passed; a container must
#     bind a non-loopback address to be reachable at all, hence the flag.

FROM node:22-alpine AS ui
WORKDIR /src/internal/dashboard/ui
COPY internal/dashboard/ui/package.json internal/dashboard/ui/package-lock.json ./
COPY internal/dashboard/ui/app/package.json app/
COPY internal/dashboard/ui/packages/mission-control/package.json packages/mission-control/
RUN npm ci
COPY internal/dashboard/ui/ ./
# vite.config.ts writes to ../../web/dist, i.e. /src/internal/dashboard/web/dist.
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Whatever the build context had under web/ (the placeholder, or a stale
# local build) is replaced by the ui stage's fresh output.
RUN rm -rf internal/dashboard/web/dist
COPY --from=ui /src/internal/dashboard/web/dist internal/dashboard/web/dist
ARG VERSION=0.1.0-dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /dream ./cmd/dream

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 1001 dream \
 && mkdir -p /data && chown 1001:1001 /data
COPY --from=build /dream /usr/local/bin/dream
COPY reporting/library /reports
USER 1001
ENTRYPOINT ["dream"]
# --insecure: see the header comment — plaintext behind a TLS proxy.
CMD ["server", "init", "--data-dir", "/data", "--addr", ":7420", "--insecure", "--reports", "/reports"]
