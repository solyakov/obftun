SHELL := /bin/bash

INSTALL_DIR := /opt/obftun

.PHONY: build arm64-build test e2e-test install-server install-tcp2tcp uninstall-server uninstall-tcp2tcp install-client clean

.DEFAULT_GOAL := build

data:
	mkdir -p $@

build: data
	go build -o data/obftun ./cmd/obftun
	go build -o data/tcp2tcp ./cmd/tcp2tcp

arm64-build: data
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o data/obftun ./cmd/obftun
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o data/tcp2tcp ./cmd/tcp2tcp

test:
	go test ./... -race -count=1

e2e-test:
	bash tests/e2e/test.sh

install-server:
	sudo install -d -m 755 $(INSTALL_DIR)
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
	-sudo rmdir $(INSTALL_DIR)
	sudo rm -f /etc/systemd/system/obftun-server.service
	sudo rm -f /etc/systemd/system/obftun-bridge.service
	sudo systemctl daemon-reload

install-client:
	install -d -m 755 $(INSTALL_DIR)
	install -m 755 scripts/ifconfig-client.sh $(INSTALL_DIR)/ifconfig-client.sh
	install -m 755 data/obftun $(INSTALL_DIR)/obftun
	install -m 755 openwrt/obftun-client.sh $(INSTALL_DIR)/obftun-client.sh
	install -m 755 openwrt/obftund /etc/init.d/obftund
	# /etc/init.d/obftund enable
	# /etc/init.d/obftund start

clean:
	rm -rf data
