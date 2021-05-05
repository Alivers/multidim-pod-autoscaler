FROM gcr.io/distroless/static:latest

ARG ARCH
COPY admission-$ARCH /admission

ENTRYPOINT ["/admission"]
CMD ["--v=4", "--stderrthreshold=info"]