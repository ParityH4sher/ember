FROM docker.io/library/ubuntu:24.04@sha256:c4a8d5503dfb2a3eb8ab5f807da5bc69a85730fb49b5cfca2330194ebcc41c7b
COPY --chmod=0555 probe.sh /ember-agent
USER 65532:65532
ENTRYPOINT ["/ember-agent"]
