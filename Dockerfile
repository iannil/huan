# huan Docker image
#
# Pre-built image published to ghcr.io/iannil/huan so downstream projects
# (e.g. zhurongshuo) can run their CI jobs in a container with huan
# pre-installed — no curl/wget/jq/tar dance to download release artifacts.
#
# Build context: this Dockerfile is consumed by .github/workflows/release.yml
# AFTER `go run ./cmd/huan release` has produced release/<version>/ artifacts.
# The release workflow copies release/<version>/huan_linux_amd64/huan into
# the build context root before `docker build`, so the COPY below finds it.
#
# Image layout:
#   /usr/local/bin/huan  — huan binary (linux/amd64)
#   /etc/ssl/certs        — CA bundle (ca-certificates package)
#   /usr/bin/git          — for git clone/push used by downstream CI
#
# Base image: alpine 3.19 (~5 MB) + git (~15 MB) + huan binary (~27 MB)
# Final image size: ~47 MB
#
# Usage in downstream CI:
#   jobs:
#     build:
#       runs-on: ubuntu-latest
#       container: ghcr.io/iannil/huan:v0.3.0
#       steps:
#         - uses: actions/checkout@v4
#         - run: huan build

FROM alpine:3.19

# Install runtime dependencies:
# - git: downstream CI needs git for actions/checkout + git push deploys
# - ca-certificates: TLS verification for huan API calls (Cloudflare, etc.)
# - tzdata: timezone data for huan's date formatting (Hugo compat)
RUN apk add --no-cache git ca-certificates tzdata && \
    update-ca-certificates

# Copy huan binary. The release workflow copies release/<version>/huan_linux_amd64/huan
# to the build context root before invoking docker build, so this COPY works.
# For local builds (development), place a freshly-built huan at the repo root
# (e.g., `go build -o huan ./cmd/huan && docker build .`).
COPY huan /usr/local/bin/huan
RUN chmod +x /usr/local/bin/huan

# Verify the binary runs. This catches architecture mismatches at build time
# rather than runtime (e.g., if linux/arm64 binary accidentally COPY'd here).
RUN huan version

# Non-root user for security. Downstream CI can override with `user: root` if
# write access to /github/workspace requires it (actions/checkout default).
RUN addgroup -S huan && adduser -S -G huan huan
USER huan

ENTRYPOINT ["huan"]
CMD ["--help"]
