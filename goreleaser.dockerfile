FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY mpas-product-controller /manager
COPY hack/entrypoint.sh /entrypoint.sh
USER 65532:65532

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/manager"]
