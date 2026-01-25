SHELL := /bin/bash

# EC2 IP address (adjust to your setup before running 'make keys')
SERVER_SAN := IP:<server-ip>

INSTALL_DIR := /opt/obftun

.PHONY: build arm64-build install-server install-tcp2tcp uninstall-server install-client keys clean

.DEFAULT_GOAL := build

data:
	mkdir -p $@

build: data
	go build -o data/obftun ./cmd/obftun
	go build -o data/tcp2tcp ./cmd/tcp2tcp

arm64-build: data
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o data/obftun ./cmd/obftun
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o data/tcp2tcp ./cmd/tcp2tcp

install-server:
	sudo install -d -m 755 $(INSTALL_DIR)
	sudo install -m 644 data/server.crt $(INSTALL_DIR)/server.crt
	sudo install -m 600 data/server.key $(INSTALL_DIR)/server.key
	sudo install -m 644 data/ca.crt $(INSTALL_DIR)/ca.crt
	sudo install -m 755 scripts/ifconfig-server.sh $(INSTALL_DIR)/ifconfig-server.sh
	sudo install -m 755 data/obftun $(INSTALL_DIR)/obftun
	sudo install -m 644 systemd/obftun-server.service /etc/systemd/system/obftun-server.service
	sudo install -m 644 systemd/obftun-bridge.service /etc/systemd/system/obftun-bridge.service
	sudo systemctl daemon-reload
	sudo systemctl enable --now obftun-bridge.service
	sudo systemctl enable --now obftun-server.service

install-tcp2tcp:
	sudo install -d -m 755 $(INSTALL_DIR)
	sudo install -m 755 data/tcp2tcp $(INSTALL_DIR)/tcp2tcp
	sudo install -m 644 systemd/tcp2tcp@.service /etc/systemd/system/tcp2tcp@.service
	sudo systemctl daemon-reload
	sudo systemctl enable --now tcp2tcp@443.service
	sudo systemctl enable --now tcp2tcp@8443.service

uninstall-tcp2tcp:
	-sudo systemctl disable --now tcp2tcp@443.service
	-sudo systemctl disable --now tcp2tcp@8443.service
	sudo rm -f $(INSTALL_DIR)/tcp2tcp
	sudo rm -f /etc/systemd/system/tcp2tcp@.service
	sudo systemctl daemon-reload

uninstall-server:
	sudo systemctl disable --now obftun-server.service
	sudo systemctl disable --now obftun-bridge.service
	sudo rm -f $(INSTALL_DIR)/obftun
	sudo rm -f $(INSTALL_DIR)/ifconfig-server.sh
	sudo rm -f $(INSTALL_DIR)/server.crt
	sudo rm -f $(INSTALL_DIR)/server.key
	sudo rm -f $(INSTALL_DIR)/ca.crt
	-sudo rmdir $(INSTALL_DIR)
	sudo rm -f /etc/systemd/system/obftun-server.service
	sudo rm -f /etc/systemd/system/obftun-bridge.service
	sudo systemctl daemon-reload

install-client:
	install -d -m 755 $(INSTALL_DIR)
	install -m 644 data/client.crt $(INSTALL_DIR)/client.crt
	install -m 600 data/client.key $(INSTALL_DIR)/client.key
	install -m 644 data/ca.crt $(INSTALL_DIR)/ca.crt
	install -m 755 scripts/ifconfig-client.sh $(INSTALL_DIR)/ifconfig-client.sh
	install -m 755 data/obftun $(INSTALL_DIR)/obftun
	install -m 755 openwrt/obftun-client.sh $(INSTALL_DIR)/obftun-client.sh
	install -m 755 openwrt/obftund /etc/init.d/obftund
	# /etc/init.d/obftund enable
	# /etc/init.d/obftund start

keys: data
	openssl ecparam -name prime256v1 -genkey -noout -out data/ca.key
	openssl req -new -x509 -nodes -sha256 -key data/ca.key -out data/ca.crt \
	    -subj "/CN=CA" \
	    -addext "basicConstraints = CA:TRUE,pathlen:0" \
	    -addext "keyUsage = critical, digitalSignature, cRLSign, keyCertSign" \
		-days 3650

	openssl ecparam -name prime256v1 -genkey -noout -out data/server.key
	openssl req -new -key data/server.key -out data/server.csr \
	    -subj "/CN=Server"
	openssl x509 -req -in data/server.csr -out data/server.crt \
		-CA data/ca.crt \
		-CAkey data/ca.key \
		-CAcreateserial \
	    -extfile <(echo "subjectAltName = $(SERVER_SAN)") \
		-days 3650

	openssl ecparam -name prime256v1 -genkey -noout -out data/client.key
	openssl req -new -key data/client.key -out data/client.csr \
	    -subj "/CN=Client"
	openssl x509 -req -in data/client.csr -out data/client.crt \
		-CA data/ca.crt \
		-CAkey data/ca.key \
		-CAcreateserial \
		-days 3650

clean:
	rm -rf data
