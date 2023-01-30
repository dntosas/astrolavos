FROM alpine as builder

# Switch to distroless as minimal base image to package the capi2argo-cluster-operator binary
FROM "gcr.io/distroless/static:nonroot"
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /
COPY bin/astrolavos .
USER 65532:65532
ENTRYPOINT ["./astrolavos"]
