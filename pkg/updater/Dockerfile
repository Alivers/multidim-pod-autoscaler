FROM gcr.io/distroless/static:latest

ARG ARCH
COPY updater-$ARCH /updater

ENTRYPOINT ["/updater"]
CMD ["--v=4", "--stderrthreshold=info"]