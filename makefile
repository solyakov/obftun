SHELL := /bin/bash
APP_NAME := obftun
DATA_DIR := data
SERVER_SAN := IP:<server-ip>
INSTALL_DIR := /opt/$(APP_NAME)
SERVICE_NAME := $(APP_NAME)-server.service
SCRIPTS_DIR := scripts
SYSTEMD_DIR := systemd

.PHONY: build arm64-build keys clean install-server uninstall-server

build: $(DATA_DIR)
	go build -o $(DATA_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

arm64-build: $(DATA_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o $(DATA_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

install-server:
	sudo install -d -m 755 $(INSTALL_DIR)
	sudo install -m 644 $(DATA_DIR)/server.crt $(INSTALL_DIR)/server.crt
	sudo install -m 644 $(DATA_DIR)/server.key $(INSTALL_DIR)/server.key
	sudo install -m 644 $(DATA_DIR)/ca.crt $(INSTALL_DIR)/ca.crt
	sudo install -m 755 $(SCRIPTS_DIR)/ifconfig-server.sh $(INSTALL_DIR)/ifconfig-server.sh
	sudo install -m 755 $(DATA_DIR)/$(APP_NAME) $(INSTALL_DIR)/$(APP_NAME)
	sudo install -m 644 $(SYSTEMD_DIR)/$(SERVICE_NAME) /etc/systemd/system/$(SERVICE_NAME)
	sudo systemctl daemon-reload
	sudo systemctl enable --now $(SERVICE_NAME)

uninstall-server:
	sudo systemctl disable --now $(SERVICE_NAME)
	sudo rm -f $(INSTALL_DIR)/$(APP_NAME)
	sudo rm -f $(INSTALL_DIR)/ifconfig-server.sh
	sudo rm -f $(INSTALL_DIR)/server.crt
	sudo rm -f $(INSTALL_DIR)/server.key
	sudo rm -f $(INSTALL_DIR)/ca.crt
	-sudo rmdir $(INSTALL_DIR)
	sudo rm -f /etc/systemd/system/$(SERVICE_NAME)
	sudo systemctl daemon-reload

$(DATA_DIR):
	mkdir -p $@

keys: $(DATA_DIR)
	openssl ecparam -name prime256v1 -genkey -noout -out $(DATA_DIR)/ca.key
	openssl req -new -x509 -nodes -sha256 -key $(DATA_DIR)/ca.key -out $(DATA_DIR)/ca.crt \
	    -subj "/CN=CA" \
	    -addext "basicConstraints = CA:TRUE,pathlen:0" \
	    -addext "keyUsage = critical, digitalSignature, cRLSign, keyCertSign" \
		-days 3650

	openssl ecparam -name prime256v1 -genkey -noout -out $(DATA_DIR)/server.key
	openssl req -new -key $(DATA_DIR)/server.key -out $(DATA_DIR)/server.csr \
	    -subj "/CN=Server"
	openssl x509 -req -in $(DATA_DIR)/server.csr -out $(DATA_DIR)/server.crt \
		-CA $(DATA_DIR)/ca.crt \
		-CAkey $(DATA_DIR)/ca.key \
		-CAcreateserial \
	    -extfile <(echo "subjectAltName = $(SERVER_SAN)") \
		-days 3650

	openssl ecparam -name prime256v1 -genkey -noout -out $(DATA_DIR)/client.key
	openssl req -new -key $(DATA_DIR)/client.key -out $(DATA_DIR)/client.csr \
	    -subj "/CN=Client"
	openssl x509 -req -in $(DATA_DIR)/client.csr -out $(DATA_DIR)/client.crt \
		-CA $(DATA_DIR)/ca.crt \
		-CAkey $(DATA_DIR)/ca.key \
		-CAcreateserial \
		-days 3650

clean:
	rm -rf $(DATA_DIR)
