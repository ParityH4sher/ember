FROM docker.io/library/golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOPROXY=off GOTOOLCHAIN=local
COPY go.mod *.go ./
RUN go test ./... && go build -trimpath -ldflags="-s -w" -o /thumbnail .

FROM build AS probe-build
RUN go test -tags qualification -c -o /probe .

FROM scratch AS probe
COPY --from=probe-build /probe /probe
USER 65532:65532
ENTRYPOINT ["/probe", "-test.run=^TestBoundary$"]

FROM scratch AS worker
COPY --from=build /thumbnail /thumbnail
USER 65532:65532
ENTRYPOINT ["/thumbnail"]
