# Build stage: only used for CA certificates
FROM alpine:3.23 AS builder

# Runtime stage: minimal distroless image
# Note: The binary is built externally (via Makefile or GoReleaser)
# and copied into this image. Run `make build` before `make docker-build`.
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /
COPY astrolavos .
USER 65532:65532
ENTRYPOINT ["./astrolavos"]
