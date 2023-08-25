FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY mpas-product-controller /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
