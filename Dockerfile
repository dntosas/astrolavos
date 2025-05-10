FROM alpine:3.21 as builder

# Switch to distroless as minimal base image to package the astrolavos binary
FROM "gcr.io/distroless/static:nonroot"
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /
COPY astrolavos .
USER 65532:65532
ENTRYPOINT ["./astrolavos"]
