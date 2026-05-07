ARG OPENBAO_HSM_BASE_IMAGE=openbao/openbao-hsm:2.5.3
FROM ${OPENBAO_HSM_BASE_IMAGE}

USER root

RUN apk add --no-cache opensc softhsm

COPY softhsm2.conf /etc/softhsm2.conf
COPY bao-softhsm-wrapper /usr/local/bin/bao-softhsm-wrapper

ENV SOFTHSM2_CONF=/etc/softhsm2.conf

RUN set -eux; \
    mkdir -p /opt/openbao-softhsm/tokens; \
    printf 'directories.tokendir = /opt/openbao-softhsm/tokens\nobjectstore.backend = file\nlog.level = ERROR\nslots.removable = false\n' > /opt/openbao-softhsm/softhsm2.conf; \
    SOFTHSM2_CONF=/opt/openbao-softhsm/softhsm2.conf \
      softhsm2-util --init-token --free --label OpenBao --so-pin 1234 --pin 1234; \
    SOFTHSM2_CONF=/opt/openbao-softhsm/softhsm2.conf \
      pkcs11-tool --module /usr/lib/softhsm/libsofthsm2.so \
        --token-label OpenBao \
        --login \
        --pin 1234 \
        --keypairgen \
        --key-type rsa:4096 \
        --label bao-root-key-rsa \
        --id 01; \
    mv /bin/bao /bin/bao.real; \
    ln -s /usr/local/bin/bao-softhsm-wrapper /bin/bao; \
    chmod 0755 /usr/local/bin/bao-softhsm-wrapper; \
    chown -R 100:1000 /opt/openbao-softhsm; \
    chmod -R u=rwX,g=rX,o= /opt/openbao-softhsm
