FROM registry.cn-shanghai.aliyuncs.com/k8s-images-aliver/distroless-static:latest

ARG ARCH
COPY cpu-bound-$ARCH /cpu-bound

ENTRYPOINT ["/cpu-bound"]
CMD ["--v=4", "--stderrthreshold=info"]