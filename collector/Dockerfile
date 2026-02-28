# Build stage: OCB compiles 70+ components (extensions, processors, exporters, etc.) into one binary.
# First build can take 45–90+ min: module download + full compile. Use BuildKit cache mounts to speed up
# subsequent builds. If you see 60+ min, ensure you're building for your host arch (no QEMU emulation).
FROM --platform=$BUILDPLATFORM golang:1.26-trixie AS builder
COPY ./collector/ /go/src/collector
# Need to copy the common folder to the collector for custom connector that uses it
COPY ./common/ /go/src/common
WORKDIR /go/src/collector

ARG TARGETARCH
ARG RHEL=false
# Set SKIP_TESTS=1 for faster local/Docker builds (e.g. make build-collector BUILD_ARGS="--build-arg SKIP_TESTS=1")
ARG SKIP_TESTS=0
# Split into two steps so you can see progress: [builder 5/6] = generate + download deps (~10–25 min), [builder 6/6] = compile (~45–70 min).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$SKIP_TESTS" = "1" ]; then \
      GOOS=linux GOARCH=$TARGETARCH make build-odigoscol-get-modules-only; \
    fi
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$SKIP_TESTS" = "1" ]; then \
      GOOS=linux GOARCH=$TARGETARCH make build-odigoscol-skip-tests; \
    else \
      GOOS=linux GOARCH=$TARGETARCH make build-odigoscol; \
    fi
RUN if [ "$RHEL" = "true" ] ; then \
      make licenses ; \
    fi

FROM registry.access.redhat.com/ubi9/ubi-micro:latest AS rhel
ARG VERSION
ARG RELEASE
ARG SUMMARY
ARG DESCRIPTION
LABEL "name"="collector"
LABEL "vendor"="Odigos"
LABEL "maintainer"="Odigos"
LABEL "version"=$VERSION
LABEL "release"=$RELEASE
LABEL "summary"=$SUMMARY
LABEL "description"=$DESCRIPTION
COPY --from=builder /go/src/collector/odigosotelcol/odigosotelcol /odigosotelcol
COPY --from=builder /go/src/collector/odigosotelcol/licenses /licenses
COPY --from=builder /go/src/collector/odigosotelcol/LICENSE /licenses/.
USER 65532:65532
CMD ["/odigosotelcol"]

FROM gcr.io/distroless/base:latest
COPY --from=builder /go/src/collector/odigosotelcol/odigosotelcol /odigosotelcol
CMD ["/odigosotelcol"]
