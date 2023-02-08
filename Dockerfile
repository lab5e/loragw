ARG ARCH=

FROM ${ARCH}alpine:latest

LABEL Author="Ståle Dahl <stalehd@lab5e.com>"
LABEL Description="LoRaWAN Span Gateway"

EXPOSE 1680/udp

VOLUME /data

# Name of the client certificate, PEM format
ENV certfile=clientcert.crt

# Name of the private key for the client certificate, PEM format
ENV keyfile=private.key

# Name of the client certificate chain, PEM format
ENV chainfile=chain.crt

# The gateway endpoint for span
ENV span_endpoint=gw.lab5e.com:6673

# Log level (debug, info, warning, error)
ENV loglevel=info

# Port for the packet forwarders
ENV forwarder_port=1680

# Log format for service (plain, console, json)
ENV logformat=plain

ARG TARGETARCH
ADD bin/loragw.linux-${TARGETARCH} /loragw

CMD /loragw \
        --log-level=${loglevel} \
        --log-type=${logformat} \
        --cert-file=/data/${certfile} \
		--chain=/data/${chainfile} \
		--key-file=/data/${keyfile} \
		--lora-connection-string=/data/loragw.db  \
		--lora-gateway-port=${forwarder_port} \
		--lora-disable-gateway-checks \
		--lora-disable-nonce-check \
		--state-file=/data/state.json \
		--span-endpoint=${span_endpoint}