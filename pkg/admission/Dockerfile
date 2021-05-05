FROM registry.cn-shanghai.aliyuncs.com/k8s-images-aliver/distroless-static:latest

ARG ARCH
COPY admission-$ARCH /admission

ENTRYPOINT ["/admission"]
CMD ["--v=4", "--stderrthreshold=info"]