# ── Stage 1: build host binary + all .so plugins ─────────────────────────────
# One builder stage guarantees host and plugins share the SAME Go toolchain
# and the SAME huan source (each plugin's go.mod does
# `replace github.com/iannil/huan => ../../`), which plugin.Open requires —
# a prebuilt binary from release tarballs + separately-built plugins always
# drift (2026-09-04: "plugin was built with a different version of package
# internal/goarch").
FROM golang:1.26-bookworm AS builder

# CGO_ENABLED=1 needs a C toolchain; git for module downloads.
RUN apt-get update && \
    apt-get install -y --no-install-recommends gcc libc6-dev git && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH}

# Build flags MUST be identical between host and plugins: Go plugins require
# the plugin .so be compiled with the same runtime package checksums as the
# host binary. Property names are currently unexported (lowercase), so a
# plugin compiled with any flags at all is incompatible with a host built
# WITHOUT trimpath/ldflags — "plugin was built with a different version of
# package internal/goarch". Avoid all flags.
RUN go build -o /huan ./cmd/huan && \
    chmod +x /huan

# Every plugin under plugins/*/ that has its own go.mod, built with
# -buildmode=plugin against the same source tree. Output name follows the
# loader convention: config key underscores ↔ filename hyphens
# (seo_injector ↔ seo-injector.so), same as scripts/build-plugins.sh.
RUN mkdir -p /plugins-out && \
    for dir in plugins/*/; do \
      if [ -f "$dir/go.mod" ]; then \
        name="$(basename "$dir")"; \
        echo "building $name -> $name.so"; \
        (cd "$dir" && go build -buildmode=plugin -o "/plugins-out/$name.so" .); \
      fi; \
    done && \
    ls -lh /plugins-out

# ── Stage 2: minimal runtime image ───────────────────────────────────────────
FROM debian:bookworm-slim

# Runtime deps: git (CI checkout), ca-certificates (TLS), bash/tar/coreutils
# (GH Actions assume GNU tar), tzdata (date formatting), curl (API calls).
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git ca-certificates bash tar coreutils tzdata curl && \
    rm -rf /var/lib/apt/lists/* && \
    update-ca-certificates

COPY --from=builder /huan /usr/local/bin/huan
RUN chmod +x /usr/local/bin/huan

# Pin the plugin lookup root. GH Actions container jobs override HOME to
# /github/home, so huan's fallback (~/.huan) would miss /root/.huan/plugins.
# HUAN_HOME must be the huan home dir ITSELF (the loader joins $HUAN_HOME
# with "plugins"), so set it to /root/.huan, not /root.
COPY --from=builder /plugins-out /root/.huan/plugins/
ENV HUAN_HOME=/root/.huan

RUN huan version

# Note: NO `USER` directive. GH Actions runner mounts workspace at /__w/
# owned by root; actions/checkout, actions/upload-pages-artifact, etc. need
# write access. For local `docker run` isolation, override with `--user`.

ENTRYPOINT ["huan"]
CMD ["--help"]
