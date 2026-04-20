FROM --platform=$BUILDPLATFORM node:20.19.0 AS deps
WORKDIR /app
COPY frontend/webapp/package.json frontend/webapp/yarn.lock ./
COPY frontend/webapp/patches ./patches
# Empty .yalc so the yalc branch below stays off in CI; for local yalc-linked ui-kit, populate frontend/webapp/.yalc before docker build.
RUN mkdir -p .yalc
# Optional sibling ui-kit copy (see scripts/embed-ui-kit-for-docker.sh + Makefile build-tag-push-ecr-image/ui).
RUN mkdir -p embedded-ui-kit
COPY frontend/docker-build-context/ui-kit/ ./embedded-ui-kit/

# yalc > embedded ui-kit > frozen lockfile (frozen only works when @odigos/ui-kit is resolvable from lockfile paths).
RUN if [ -d .yalc ] && [ "$(ls -A .yalc 2>/dev/null)" ]; then \
      echo "Using yalc"; \
      npm install -g yalc; \
      yarn install; \
    elif [ -f ./embedded-ui-kit/package.json ]; then \
      echo "Using embedded @odigos/ui-kit"; \
      node -e "const fs=require('fs');const p=require('./package.json');p.dependencies['@odigos/ui-kit']='file:./embedded-ui-kit';fs.writeFileSync('package.json',JSON.stringify(p,null,2)+'\n');" && \
      yarn install --network-timeout 600000 && \
      (cd embedded-ui-kit && yarn install --ignore-engines --network-timeout 600000 && \
        if [ -n "$(ls -A lib 2>/dev/null)" ]; then echo "embedded ui-kit: lib/ present, skip yarn build"; else yarn build; fi); \
    else \
      echo "Using frozen lockfile"; \
      yarn install --frozen-lockfile; \
    fi

FROM --platform=$BUILDPLATFORM node:20.19.0 AS builder
WORKDIR /webapp
COPY --from=deps /app/package.json /app/yarn.lock ./
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/patches ./patches
COPY --from=deps /app/embedded-ui-kit ./embedded-ui-kit
COPY frontend/webapp .
# Restore dependency paths resolved in deps (host package.json still says file:../../../ui-kit).
COPY --from=deps /app/package.json ./package.json
COPY --from=deps /app/yarn.lock ./yarn.lock
RUN yarn build

FROM --platform=$BUILDPLATFORM golang:1.26.2 AS backend
WORKDIR /app
COPY . .
RUN rm -rf frontend/webapp/*
COPY --from=builder /webapp/out frontend/webapp/out
WORKDIR /app/frontend
ARG TARGETARCH
ARG LD_FLAGS
ARG RHEL=false
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -ldflags="${LD_FLAGS}" -o odigos-ui
RUN if [ "$RHEL" = "true" ] ; then \
      make licenses ; \
    fi

FROM registry.access.redhat.com/ubi9/ubi-micro:latest AS rhel
ARG VERSION
ARG RELEASE
ARG SUMMARY
ARG DESCRIPTION
LABEL "name"="frontend"
LABEL "vendor"="Odigos"
LABEL "maintainer"="Odigos"
LABEL "version"=$VERSION
LABEL "release"=$RELEASE
LABEL "summary"=$SUMMARY
LABEL "description"=$DESCRIPTION
WORKDIR /app
COPY --from=backend /app/frontend/odigos-ui .
COPY --from=backend /app/frontend/licenses /licenses
COPY --from=backend /app/frontend/LICENSE /licenses/.
USER 65532:65532
ENTRYPOINT ["/app/odigos-ui", "--address", "0.0.0.0"]

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=backend /app/frontend/odigos-ui .
USER 65532:65532
ENTRYPOINT ["/app/odigos-ui", "--address", "0.0.0.0"]
