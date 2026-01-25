# obftun

A secure tunnel over TLS with mutual authentication, supporting multiple concurrent clients.

## Overview

`obftun` creates encrypted Layer 2 tunnels between clients and server using TLS. Each client connection gets its own dedicated TAP interface on the server. Both client and server authenticate each other using certificates (mTLS).

### Architecture

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
│  │            Bridge (10.10.0.1)   ┌────────┼───┼──── Internet (Country 2)
│  └─────────────────────────────────┼────────┘   │
│                                    │            │
│                                   tap0          │
│                                    │            │
│                            ┌───────┴───────┐    │
│                            │ obftun-server │    │
│                            └───────┬───────┘    │
│                                    │            │
└────────────────────────────────────┼────────────┘
                                     │
                      (mTLS)         │
           ┌─────────────────────────┘
           │
┌──────────┼──────────────────────────────────────┐
│          │       OpenWrt Router                 │
│          │                                      │
│  ┌───────┴───────┐                              │
│  │ obftun-client │                              │
│  └───────┬───────┘                              │
│          │                                      │
|         tap0                                    │
|          │                        ┌─────────────┼──── Internet (Country 1)
|  ┌───────┴────────┐               |             │
|  │ Bridge (no IP) |               |             │
|  └───────┬────────┘               |             │
|          │                        │             │
|      phy1-ap0                 phy0-ap0          │
└──────────┼────────────────────────┼─────────────┘
           |                        |
     5G WiFi Clients        2.4G WiFi Clients
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
make build install-server
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
  -i, --iface           Interface name pattern (default: tap%d)
  -s, --script          Setup script for interface configuration
  -t, --script-timeout  Script execution timeout in seconds (default: 15)
  -r, --read-timeout    Connection read timeout in seconds (default: 60)
  -c, --certificate     Certificate file (default: cert.crt)
  -k, --key             Private key file (default: key.pem)
  -a, --ca              CA certificate file (default: ca.crt)
  -f, --fake            Server proxies requests to this domain for unauthenticated clients. Client uses this domain for SNI. (default: example.com)
  -v, --verbose         Verbose logging
  -p, --padding         Enable packet padding for traffic obfuscation
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
- `OBFTUN_FAKE`
- `OBFTUN_VERBOSE`
- `OBFTUN_PADDING`

## Bypassing IP Blacklists with tcp2tcp

If your obftun server gets blacklisted by ISPs, you can deploy `tcp2tcp` on an intermediate non-blacklisted server to forward traffic transparently:

```
Client (Country 1) --+-- [blocked] --> Internet (Country 2)
                     |
                     +-- [blocked] --> Obftun Server (Country 2) --> Internet (Country 2)
                     |
                     +-- [not blocked] --> tcp2tcp Server (Country 1) --> Obftun Server (Country 2) --> Internet (Country 2)
```

This works because ISPs typically only block client-to-server traffic (i.e, Client <--> Obftun Server), not inter-server traffic (i.e, tcp2tcp Server <--> Obftun Server).

Command line flags:
```
  -b, --bind            Bind address (default: :443)
  -t, --target          Target obftun server address (required)
  -v, --verbose         Verbose logging
```

All flags can be set via environment variables:
- `TCP2TCP_BIND`
- `TCP2TCP_TARGET`
- `TCP2TCP_VERBOSE`
