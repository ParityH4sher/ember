# Offline chat fixture for Ember project chat. The pinned Go builder image is
# reused because it already provides Python 3 (as in ../pilot); no model, no
# network, no credentials.
FROM docker.io/library/golang:1.27.4@sha256:205c7636d74e0ffbe3d007162df7d74f859f5a8c8322dfb9f907977686c126cf
COPY --chmod=0555 agent.py /ember-agent
RUN python3 --version
USER 65532:65532
ENTRYPOINT ["/ember-agent"]
