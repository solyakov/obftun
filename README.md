# obftun

Encrypted Layer 2 tunnel hidden inside plain HTTP traffic, designed to bypass DPI and censorship.

## Overview

`obftun` creates encrypted TAP tunnels between clients and server. Traffic is disguised as normal HTTP communication (JSON API with SSE) on port 80. A shared secret provides encryption (AES-256-GCM).

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
│                            │   (port 80)   │    │
│                            └───────┬───────┘    │
│                                    │            │
└────────────────────────────────────┼────────────┘
                                     │
                       (HTTP)        │
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

### 1. Build

```bash
make build              # Local build (for development)
make arm64-build        # For OpenWrt/ARM64 routers
```

### 2. Server Installation

Install on your server (EC2, VPS, etc.):

```bash
make build install-server
```

Edit `/etc/systemd/system/obftun-server.service` and set `OBFTUN_SECRET` to your shared secret.

Edit `/etc/systemd/system/obftun-bridge.service` to configure the bridge:
- `BRIDGE_IP` - Bridge IP address (default: 10.10.0.1)
- `EXTERNAL_IFACE` - Your internet-facing interface (e.g., ens5, eth0)
- `DNS_SERVER_1`, `DNS_SERVER_2` - DNS servers advertised to clients (default: 8.8.8.8, 8.8.4.4)

### 3. Client Installation (OpenWrt)

On your OpenWrt router:

```bash
# Copy files to router (from build machine)
scp data/obftun root@router:/opt/obftun/
scp scripts/ifconfig-client.sh root@router:/opt/obftun/
scp openwrt/obftun-client.sh root@router:/opt/obftun/
scp openwrt/obftund root@router:/etc/init.d/

# Edit client configuration
vi /opt/obftun/obftun-client.sh
# Set: dial="your-server-ip:80"
# Set: secret="your-shared-secret"
# Set: wifi_iface="phy1-ap0"  # Your WiFi interface

# Enable and start service
/etc/init.d/obftund enable
/etc/init.d/obftund start
```

## Configuration

### Server flags

```
  -b, --bind            Bind address (default: :80)
  -x, --secret          Shared secret for encryption (required)
  -m, --max-clients     Maximum concurrent clients (default: 10)
  -i, --iface           Interface name pattern (default: tap%d)
  -s, --script          Setup script for interface configuration
  -t, --script-timeout  Script execution timeout in seconds (default: 15)
  -v, --verbose         Verbose logging
```

### Client flags

```
  -d, --dial            Server address to connect to (required)
  -x, --secret          Shared secret for encryption (required)
  -i, --iface           Interface name pattern (default: tap%d)
  -s, --script          Setup script for interface configuration
  -t, --script-timeout  Script execution timeout in seconds (default: 15)
  -v, --verbose         Verbose logging
```

All flags can be set via environment variables (`OBFTUN_BIND`, `OBFTUN_DIAL`, `OBFTUN_SECRET`, etc.).

## Testing

### Unit tests

```bash
go test ./... -race
```

### End-to-end tests

Requires two Vagrant VMs (VMware Fusion):

```bash
bash tests/e2e/test.sh
```

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
