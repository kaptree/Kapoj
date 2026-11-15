FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN mkdir -p /out/data

FROM debian:bookworm-slim
RUN groupadd -r -g 65532 kapoj && useradd -r -g 65532 -u 65532 kapoj \
    && mkdir -p /data && chown kapoj:kapoj /data
COPY --from=build /out/server /server
# docker-compose.yml bind-mounts the host's apps/server/data over /data.
# Keep a writable directory in the image for runs without that mount.
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
