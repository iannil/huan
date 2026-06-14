# huan Docker image
#
# Pre-built image published to ghcr.io/iannil/huan so downstream projects
# (e.g. zhurongshuo) can run their CI jobs in a container with huan
# pre-installed — no curl/wget/jq/tar dance to download release artifacts.
#
# Build context: this Dockerfile is consumed by .github/workflows/release.yml
# AFTER `go run ./cmd/huan release` has produced release/<version>/ artifacts.
# The release workflow extracts release/<version>/huan_<version>_linux_amd64.tar.gz
# and copies the binary into the build context root before `docker build`,
# so the COPY below finds it.
#
# Base image: debian:bookworm-slim (not alpine)
# We use debian instead of alpine because:
# - GH Actions actions/upload-pages-artifact@v3 uses `tar --hard-dereference`
#   which is a GNU tar option NOT supported by busybox tar (alpine default).
# - Alpine's busybox quirks (no bash by default, limited tar flags, etc.)
#   cause repeated CI failures with various GH Actions actions.
# - debian:bookworm-slim has GNU coreutils + bash + tar pre-installed, so
#   all GH Actions ecosystem assumptions hold.
# - Size trade-off: ~80MB base + git (~25MB) + huan binary (~27MB) ≈ ~135MB.
#   Larger than alpine (41MB) but eliminates a class of CI breakage.
#
# Usage in downstream CI:
#   jobs:
#     build:
#       runs-on: ubuntu-latest
#       container: ghcr.io/iannil/huan:v0.3.0
#       steps:
#         - uses: actions/checkout@v4
#         - run: huan build

FROM debian:bookworm-slim

# Install runtime dependencies:
# - git: downstream CI needs git for actions/checkout + git push deploys
# - ca-certificates: TLS verification for huan API calls (Cloudflare, etc.)
# - bash: required by GH Actions workflows (defaults.run.shell: bash)
# - GNU tar + coreutils: required by actions/upload-pages-artifact@v3
#   (`tar --hard-dereference` is GNU-specific)
# - tzdata: timezone data for huan's date formatting (Hugo compat)
# - curl: some GH Actions actions use it for internal API calls
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git ca-certificates bash tar coreutils tzdata curl && \
    rm -rf /var/lib/apt/lists/* && \
    update-ca-certificates

# Copy huan binary. The release workflow extracts
# release/<version>/huan_<version>_linux_amd64.tar.gz and copies the
# resulting binary to the build context root before invoking docker build,
# so this COPY works.
# For local builds (development), place a freshly-built huan at the repo
# root (e.g., `go build -o huan ./cmd/huan && docker build .`).
COPY huan /usr/local/bin/huan
RUN chmod +x /usr/local/bin/huan

# Verify the binary runs. This catches architecture mismatches at build time
# rather than runtime (e.g., if linux/arm64 binary accidentally COPY'd here).
RUN huan version

# Note: NO `USER` directive. GH Actions runner mounts workspace at /__w/
# owned by root; actions/checkout, actions/upload-pages-artifact, etc. need
# write access. Setting USER to a non-root account (e.g. huan) breaks these
# actions with EACCES on /__w/_temp/_runner_file_commands/*. For local
# `docker run` use cases that need isolation, override with `--user`.

ENTRYPOINT ["huan"]
CMD ["--help"]
