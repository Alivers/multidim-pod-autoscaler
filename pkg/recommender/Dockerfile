FROM registry.cn-shanghai.aliyuncs.com/k8s-images-aliver/distroless-static:latest

ARG ARCH
COPY recommender-$ARCH /recommender

ENTRYPOINT ["/recommender"]
CMD ["--v=4", "--stderrthreshold=info", "--prometheus-address=http://prometheus-k8s.monitoring.svc:9090"]