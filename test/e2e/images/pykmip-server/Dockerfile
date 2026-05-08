ARG PYKMIP_BASE_IMAGE=python:3.11-slim
FROM ${PYKMIP_BASE_IMAGE}

ARG PYKMIP_VERSION=0.10.0

ENV PYTHONDONTWRITEBYTECODE=1

RUN python -m pip install --no-cache-dir "PyKMIP==${PYKMIP_VERSION}" && \
    mkdir -p /var/lib/pykmip && \
    chown -R 1000:1000 /var/lib/pykmip

COPY pykmip-test-server /usr/local/bin/pykmip-test-server

RUN chmod 0555 /usr/local/bin/pykmip-test-server

USER 1000:1000

EXPOSE 5696
ENTRYPOINT ["/usr/local/bin/pykmip-test-server"]
