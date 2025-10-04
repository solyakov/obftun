# obftun

A secure point-to-point TUN tunnel over TLS with mutual authentication.

## Overview

`obftun` creates an encrypted IP tunnel between two machines using TLS. Both client and server authenticate each other using certificates (mTLS).

```
        ┌─────────────────────┐                                ┌─────────────────────┐
        │      Client         │                                │      Server         │
        │                     │                                │                     │
        │  ┌──────────────┐   │                                │   ┌──────────────┐  │
        │  │   tun0       │   │         TLS Connection         │   │   tun0       │  │
  LAN ◄─┼─►│ 10.10.0.2/24 │◄──┼────────────────────────────────┼──►│ 10.10.0.1/24 │◄─┼─► Internet
        │  └──────────────┘   │     (mutual auth via certs)    │   └──────────────┘  │
        │                     │                                │                     │
        └─────────────────────┘                                └─────────────────────┘
            OpenWrt Router                 Internet                      EC2
```

## How It Works

1. Both sides create TUN interfaces
2. Setup script configures IP addresses and routing
3. TLS connection with mutual certificate authentication
4. IP packets are length-framed and encrypted over TCP
5. Traffic flows bidirectionally through the secure tunnel

## Quick Start

### 1. Generate Certificates

```bash
make keys
```

### 2. Build

```bash
make build              # Local build
make arm64-build        # For OpenWrt/ARM64
```

### 3. Run Server

```bash
./obftun \
  --bind :8443 \
  --certificate data/server.crt \
  --key data/server.key \
  --ca data/ca.crt \
  --script scripts/ifconfig-server.sh
```

### 4. Run Client

```bash
./obftun \
  --dial <server-ip>:8443 \
  --certificate data/client.crt \
  --key data/client.key \
  --ca data/ca.crt \
  --script scripts/ifconfig-client.sh
```

## Installation

### Server (systemd)

```bash
make keys
make build
make install-server
systemctl status obftun-server
```

## Configuration

Flags can be set via command line or environment variables:

```
  -b, --bind           Server bind address (e.g., :8443)
  -d, --dial           Server address to connect to
  -i, --iface          TUN interface name (default: obftun)
  -s, --script         Setup script for interface configuration
  -c, --certificate    Certificate file
  -k, --key            Private key file
  -a, --ca             CA certificate file
  -v, --verbose        Verbose logging
```

Environment variables: `OBFTUN_BIND`, `OBFTUN_DIAL`, etc.

## Security

While the content is encrypted, ISPs can still see that you are using a TLS tunnel. Future versions may add obfuscation layers if TLS alone is no longer sufficient for me.
