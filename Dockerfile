ARG ARCH=

FROM ${ARCH}alpine:latest

LABEL Author="Ståle Dahl <stalehd@lab5e.com>"
LABEL Description="LoRaWAN Span Gateway"

EXPOSE 1680/udp

VOLUME /data

ENV certfile=clientcert.crt
ENV keyfile=private.key
ENV chainfile=chain.crt
ENV span_endpoint=gw.lab5e.com:6673
ENV loglevel=info
ENV forwarder_port=1680

ARG TARGETARCH
ADD bin/loragw.linux-${TARGETARCH} /loragw
CMD /loragw \
        --log-level=${loglevel} \
        --cert-file=/data/${certfile} \
		--chain=/data/${chainfile} \
		--key-file=/data/${keyfile} \
		--lora-connection-string=/data/loragw.db  \
		--lora-gateway-port=${forwarder_port} \
		--lora-disable-gateway-checks \
		--lora-disable-nonce-check \
		--state-file=/data/state.json \
		--span-endpoint=${span_endpoint}