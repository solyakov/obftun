# -*- mode: ruby -*-
# vi: set ft=ruby :

GO_VERSION = "1.24.0"

PROVISION_GO = <<-SHELL
  set -e
  if ! command -v go &>/dev/null || ! go version | grep -q "#{GO_VERSION}"; then
    echo "Installing Go #{GO_VERSION}..."
    ARCH=$(dpkg --print-architecture)
    curl -fsSL "https://go.dev/dl/go#{GO_VERSION}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
  fi
  export PATH=$PATH:/usr/local/go/bin
  go version
SHELL

PROVISION_SERVER = <<-SHELL
  set -e

  apt-get update -qq
  apt-get install -y -qq dnsmasq iperf3 jq > /dev/null
  systemctl stop dnsmasq 2>/dev/null || true
  systemctl disable dnsmasq 2>/dev/null || true
  systemctl stop iperf3 2>/dev/null || true
  systemctl disable iperf3 2>/dev/null || true

  # Create bridge for tunnel interfaces
  if ! ip link show br-obftun &>/dev/null; then
    ip link add name br-obftun type bridge
    ip addr add 10.10.0.1/16 dev br-obftun
    ip link set br-obftun up
  fi

  echo 1 > /proc/sys/net/ipv4/ip_forward

  # Build obftun
  cd /vagrant
  export PATH=$PATH:/usr/local/go/bin
  go build -o /usr/local/bin/obftun ./cmd/obftun/
  cp scripts/ifconfig-server.sh /usr/local/bin/ifconfig-server.sh
  chmod +x /usr/local/bin/ifconfig-server.sh

  echo "Server provisioned."
SHELL

PROVISION_CLIENT = <<-SHELL
  set -e

  apt-get update -qq
  apt-get install -y -qq isc-dhcp-client iperf3 jq > /dev/null

  # Create bridge for tunnel interface
  if ! ip link show br-client &>/dev/null; then
    ip link add name br-client type bridge
    ip link set br-client up
  fi

  # Build obftun
  cd /vagrant
  export PATH=$PATH:/usr/local/go/bin
  go build -o /usr/local/bin/obftun ./cmd/obftun/
  cp scripts/ifconfig-client.sh /usr/local/bin/ifconfig-client.sh
  chmod +x /usr/local/bin/ifconfig-client.sh

  echo "Client provisioned."
SHELL

Vagrant.configure("2") do |config|
  config.vm.box = "bento/ubuntu-24.04"
  config.vm.box_version = "202404.26.0"
  config.vm.synced_folder ".", "/vagrant", type: "rsync",
    rsync__exclude: [".git/", "*.test"]

  config.vm.provider "vmware_desktop" do |v|
    v.vmx["memsize"] = "512"
    v.vmx["numvcpus"] = "2"
  end

  # --- Server VM ---
  config.vm.define "server" do |s|
    s.vm.hostname = "obftun-server"
    s.vm.provision "shell", inline: PROVISION_GO
    s.vm.provision "shell", inline: PROVISION_SERVER
  end

  # --- Client VM ---
  config.vm.define "client" do |c|
    c.vm.hostname = "obftun-client"
    c.vm.provision "shell", inline: PROVISION_GO
    c.vm.provision "shell", inline: PROVISION_CLIENT
  end
end
