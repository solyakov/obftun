# obftun

A secure tunnel over TLS with mutual authentication, supporting multiple concurrent clients.

## Overview

`obftun` creates encrypted Layer 2 tunnels between clients and server using TLS. Each client connection gets its own dedicated TAP interface on the server. Both client and server authenticate each other using certificates (mTLS).

### Server Architecture

```
┌─────────────────────────────────────────────────┐
│                   Server Host                   │
│                                                 │
│  ┌────────────────┐                             │
│  │ DHCP Server    │                             │
│  │ 10.10.0.10-254 │                             │
│  └───┬────────────┘                             │
│      |                                          │
│  ┌───┴──────────────────────────────────────┐   │
│  │            Bridge (10.10.0.1)            │   │
│  │           (L2 Ethernet switch)           │   │
│  └───┬──────┬──────┬──────┬─────────────────┘   │
│      │      │      │      │                     │
│    tap0   tap1   tap2   tap3  ...               │
│      │      │      │      │                     │
└──────┼──────┼──────┼──────┼─────────────────────┘
       │      │      │      │
     (TLS)  (TLS)  (TLS)  (TLS)
       │      │      │      │
       Clients over Internet
```

### Client Architecture (OpenWrt)

```
┌─────────────────────────────────────────────────┐
│                OpenWrt Router                   │
│                                                 │
│  ┌──────────────────────────────────────────┐   │
│  │           Bridge (br-client)             │   │
│  │         (no IP, pure L2 switch)          │   │
│  └───┬──────────────────────────────────┬───┘   │
│      │                                  │       │
│  ┌───┴────┐                        ┌────┴────┐  │
│  │  WiFi  │                        │   TAP   │  │
│  │phy1-ap0│                        │  (tap0) │  │
│  │(static)│                        │(dynamic)│  │
│  └───┬────┘                        └────┬────┘  │
│      │                                  │       │
└──────┼──────────────────────────────────┼───────┘
       │                                  |
  WiFi Clients                       TLS to Server
```

## Quick Start

### 1. Generate Certificates

Edit `makefile` and set `SERVER_SAN` to your server's IP address:
```makefile
SERVER_SAN := IP:1.2.3.4
```

Then generate certificates:
```bash
make keys
```

This creates:
- Certificate Authority: `data/ca.crt` + `data/ca.key`
- Server certificate: `data/server.crt` + `data/server.key`
- Client certificate: `data/client.crt` + `data/client.key`

### 2. Build

```bash
make build              # Local build (for development)
make arm64-build        # For OpenWrt/ARM64 routers
```

### 3. Server Installation

Install on your server (EC2, VPS, etc.):

```bash
GOOS=linux go build -o data/obftun ./cmd/obftun
make install-server
```

This installs:
- Binary: `/opt/obftun/obftun`
- Interface management script: `/opt/obftun/ifconfig-server.sh`
- Server certificates: `/opt/obftun/server.{crt,key}`
- CA certificate: `/opt/obftun/ca.crt`
- Bridge service: `/etc/systemd/system/obftun-bridge.service`
- Server service: `/etc/systemd/system/obftun-server.service`

Edit `/etc/systemd/system/obftun-bridge.service` to configure the bridge:
- `BRIDGE_IP` - Bridge IP address (default: 10.10.0.1)
- `EXTERNAL_IFACE` - Your internet-facing interface (e.g., ens5, eth0)
- `DNS_SERVER_1`, `DNS_SERVER_2` - DNS servers advertised to clients (default: 8.8.8.8, 8.8.4.4)

### 4. Client Installation (OpenWrt)

On your OpenWrt router:

```bash
# Copy files to router (from build machine)
scp data/obftun root@router:/opt/obftun/
scp scripts/ifconfig-client.sh root@router:/opt/obftun/
scp data/client.{crt,key} data/ca.crt root@router:/opt/obftun/
scp openwrt/obftun-client.sh root@router:/opt/obftun/
scp openwrt/obftund root@router:/etc/init.d/

# Edit client configuration
vi /opt/obftun/obftun-client.sh
# Set: dial="your-server-ip:8443"
# Set: wifi_iface="phy1-ap0"  # Your WiFi interface

# Enable and start service
/etc/init.d/obftund enable
/etc/init.d/obftund start
```

## Configuration

Command line flags:
```
  -b, --bind            Server bind address (default: :8443 for server)
  -d, --dial            Server address to connect to (client only)
  -i, --iface           TAP interface name pattern (default: tap%d)
  -s, --script          Setup script for interface configuration
  -t, --script-timeout  Script execution timeout in seconds (default: 15)
  -r, --read-timeout    Connection read timeout in seconds (default: 60)
  -c, --certificate     Certificate file (default: cert.crt)
  -k, --key             Private key file (default: key.pem)
  -a, --ca              CA certificate file (default: ca.crt)
  -v, --verbose         Verbose logging
```

All flags can be set via environment variables:
- `OBFTUN_BIND`
- `OBFTUN_DIAL`
- `OBFTUN_IFACE`
- `OBFTUN_SCRIPT`
- `OBFTUN_SCRIPT_TIMEOUT`
- `OBFTUN_READ_TIMEOUT`
- `OBFTUN_CERTIFICATE`
- `OBFTUN_KEY`
- `OBFTUN_CA`
- `OBFTUN_VERBOSE`

