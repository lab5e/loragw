# LoRaWAN Gateway process

## REST API

### Create and launch gateway

```shell
# Create the new gateway. Let Span assign the app key and network key
curl -XPOST -d '{"type":"lora"}' api.lab5e.com/span/gateways

# Download the zip file
curl api.lab5e.com/span/gateways/{newid}/package

# Unzip and launch the gateway
unzip lora.zip

# File includes loragw, cert.crt, key.pem, config
./loragw
```

### Create a new device
```shell

# Create a new device
loracli device new 

# Get the generated code for the device
loracli device code --language=c

# Build device, should appear in Span
```

### Manage devices

```shell
# Get the device 
curl api.lab5e.com/span/collections/{id}/devices/{id}

# Disable device
curl -XPATCH -d '{"enabled": "false"}' api.lab5e.com/span/collections/{id}/devices/{id}
```

### Downstream messages

Downstream messages are handled asynchronously. Send acks are sent back to span (and must be acked by Span).
Messages have state "pending" until they have been acked and status is either "sent" or "error"
```shell
# Queue message. This is sent to the gateway. Status is set as "pending" until the gateway confirms it
curl api.lab5e.com/span/collections/{id}/devices/{id}/outbox -XPATCH -d '{..., "transport": "gw"}'
```

#### Logging

Gateway logs are sent back to Span. This includes diagnostic messsages

#### State

Gateway state is sent to Span ("online", "offline") - ie connections

### Connection

One connection per gateway. gRPC. Gateway connects to Span. Two command streams: gRPC stream message for downstream commands.
gRPC stream message for upstream notifications.

Alternative: DTLS w/ framing for gateway + protobuf messages?

Gateway should be online most of the time.

### Gateway modes

Connected mode - always online. Use certificate to encrypt and authenticate.
Disconnected mode - bursts with upstream and downstream data. Suited for packet transport. (DTLS)

### Event log

DeviceCreated - device removed
DeviceOnline - session start for device
DeviceOffline - session end for device
DeviceMessage - upstream message for device
DeviceConfig - config change for device

### Configuration

* State
* AutoCreateDevice
* ConfigChange


### Span changes

Schema for igw/ciotgw/lora/custom

Custom schema is map of strings. 

IGW and APN are fixed and read-only

#### Long term APN and IGW

Gateway implementations. Launch as separate processes.

