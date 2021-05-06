FROM registry.cn-shanghai.aliyuncs.com/k8s-images-aliver/distroless-static:latest

ARG ARCH
COPY recommender-$ARCH /recommender

ENTRYPOINT ["/recommender"]
CMD ["--v=4", "--stderrthreshold=info"]