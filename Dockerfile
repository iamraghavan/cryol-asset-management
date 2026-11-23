FROM node:22-bookworm-slim AS web
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
RUN corepack pnpm install --frozen-lockfile
COPY apps/web apps/web
RUN corepack pnpm build

FROM golang:1.26.4-bookworm AS server
WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download
COPY apps/server/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/cryol-server ./cmd/cryol-server && \
    CGO_ENABLED=0 go build -trimpath -o /out/cryol-cli ./cmd/cryol-cli

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl openssl && \
    rm -rf /var/lib/apt/lists/* && \
    install -d -o 10001 -g 10001 -m 0700 /var/lib/cryol/tls && \
    touch /var/lib/cryol/tls/.initialized
COPY --from=server /out/ /usr/local/bin/
COPY --from=web /src/apps/web/dist/ /opt/cryol/web/
COPY --chmod=0755 scripts/cryol-dev-certificate.sh /usr/local/bin/cryol-dev-certificate
USER 10001:10001
EXPOSE 8443
CMD ["cryol-server"]
