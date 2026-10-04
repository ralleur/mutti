# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM node:26.7.0-bookworm-slim@sha256:4db36457f406501e6f608802e5da617e5fbd0e80b75901b6a09de1ae5a667d32 AS web-build
WORKDIR /web
COPY --from=web package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY --from=web . .
RUN npm run build:production

FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:10.0.401@sha256:e70cdb7f80b0348f5cb85f19a8f670fca061f033d57eed12fa003d58b0e06317 AS server-build
ARG TARGETARCH
ENV DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1
WORKDIR /src
COPY . .
RUN case "$TARGETARCH" in arm64) RID=linux-arm64;; amd64) RID=linux-x64;; *) exit 1;; esac \
    && dotnet publish Jellyfin.Server/Jellyfin.Server.csproj -c Release -r "$RID" --self-contained true -o /out

FROM jellyfin/jellyfin:12.1@sha256:78d3ea1207d1322471fcac39a614f004f2ccf7e878f95ab2977d752f07e4dd7e
LABEL org.opencontainers.image.title="Mutti development preview" \
      org.opencontainers.image.source="https://github.com/ralleur/mutti" \
      org.opencontainers.image.licenses="GPL-2.0-or-later"
COPY --from=server-build /out /jellyfin/
COPY --from=web-build /web/dist /jellyfin/jellyfin-web/
COPY mutti/packaging/docker/entrypoint.sh /mutti-entrypoint.sh
COPY mutti/components.lock.json mutti/THIRD-PARTY.md /mutti/
USER 1000:1000
ENV JELLYFIN_DATA_DIR=/config/data JELLYFIN_CONFIG_DIR=/config/config JELLYFIN_LOG_DIR=/config/log JELLYFIN_CACHE_DIR=/cache \
    MUTTI_PREVIEW_ORIGIN=http://127.0.0.1:18597
HEALTHCHECK --interval=10s --timeout=3s --start-period=60s CMD curl --fail --silent -H 'Host: 127.0.0.1:18597' http://127.0.0.1:8096/health || exit 1
ENTRYPOINT ["/mutti-entrypoint.sh"]
