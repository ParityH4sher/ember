# Offline acceptance fixture. The Go builder image also provides Git and Python 3.
FROM docker.io/library/golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf
COPY --chmod=0555 entrypoint.py /ember-agent
COPY --chmod=0555 fixture-task.sh /usr/local/bin/ember-task
RUN git --version && python3 --version
USER 65532:65532
ENTRYPOINT ["/ember-agent"]
