# Switch to distroless as minimal base image to package the capi2argo-cluster-operator binary
FROM "gcr.io/distroless/static:nonroot"
WORKDIR /
COPY bin/astrolavos .
USER 65532:65532
ENTRYPOINT ["./astrolavos"]
