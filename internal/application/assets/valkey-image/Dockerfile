FROM docker.io/valkey/valkey:9.1.2-alpine@sha256:48332870af354a799964c0012ae1194a0bf2bf894eb508f945810596dc2d8d11

USER root
RUN chgrp -R 0 /data \
    && chmod -R g=u /data \
    && chmod 2770 /data
USER valkey:0
