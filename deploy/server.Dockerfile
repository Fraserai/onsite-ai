# Build context is the repository root.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && mkdir -p /out/data /out/public

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
# Created here so the named volumes mounted over them start out writable by
# the non-root user.
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build --chown=65532:65532 /out/public /public
USER 65532:65532
EXPOSE 8443
ENTRYPOINT ["/server"]
