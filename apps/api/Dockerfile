FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ember-api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ember-api /ember-api
EXPOSE 8080
ENTRYPOINT ["/ember-api"]
