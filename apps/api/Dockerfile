# The build stage runs on the build machine and cross-compiles, so multi-arch
# images (amd64, arm64) need no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/ember-api .
RUN mkdir -p /out/projects /out/ai

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/ember-api /ember-api
COPY --from=build --chown=65532:65532 /out/projects /var/lib/ember/projects
# Providers, models and agents admins add in the web app; holds provider keys.
COPY --from=build --chown=65532:65532 /out/ai /var/lib/ember/ai
ENV EMBER_PROJECT_ROOT=/var/lib/ember/projects
ENV EMBER_AI_CATALOG_FILE=/var/lib/ember/ai/catalog.json
EXPOSE 8080
ENTRYPOINT ["/ember-api"]
