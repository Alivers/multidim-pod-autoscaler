FROM registry.cn-shanghai.aliyuncs.com/k8s-images-aliver/distroless-static:latest

ARG ARCH
COPY updater-$ARCH /updater

ENTRYPOINT ["/updater"]
CMD ["--v=4", "--stderrthreshold=info"]