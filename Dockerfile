ARG ARCH=

FROM ${ARCH}alpine:latest

LABEL Author="Ståle Dahl <stalehd@lab5e.com>"
LABEL Description="LoRaWAN Span Gateway"

EXPOSE 1680/udp

VOLUME /data

ENV certfile=clientcert.crt
ENV keyfile=private.key
ENV chainfile=chain.crt
ENV span_endpoint=
ENV loglevel=info
ENV forwarder_port=1680

RUN /loragw \
        --log-level=${loglevel} \
        --cert-file=${certfile} \
		--chain=${chainfile} \
		--key-file=${keyfile} \
		--lora-connection-string=/data/loragw.db  \
		--lora-gateway-port=${forwarder_port} \
		--lora-disable-gateway-checks \
		--lora-disable-nonce-check \
		--state-file=/data/state.json \
		--span-endpoint=${span_endpoint}