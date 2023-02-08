# Span LoRaWAN gateway image

![Lab5e logo](https://lab5e.com/images/lab5e_512x256_c.svg)

This is an image with a LoRaWAN gateway. 

Create a gateway at https://span.lab5e.com/ and generate client certificates for the gateway. 

Rename the client certificate to `clientcert.crt`, private key to `private.key` and certificate chain to `chain.crt` and place it in the current directory. 

Run with `docker run -v $(pwd):/data lab5e/loragw:v1.0.1`

Add custom parameters with the `-e [name]=[value]` on the command line.

The gateway configuration is stored in the two files `state.json` and `loragw.db`. Remove these files to reset the gateway.

## Configuration parameters

```dockerfile
# The client certificate file for the gateway
ENV certfile=clientcert.crt

# The private key for the gateway
ENV keyfile=private.key

# The certificate chain for the client certificate
ENV chainfile=chain.crt

# The log level to use (debug, info, warning, error)
ENV loglevel=info

# The log format (plain, console, json)
ENV logformat=plain

# The packet forwarder port to listen to. The gateway EUI or source isn't checked; all connections are
# accepted
ENV forwarder_port=1680
```

> Note: This is not a production ready LoRaWAN server. It is better suited for development and testing.