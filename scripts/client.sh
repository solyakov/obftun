#!/bin/bash

cd $(dirname $0)

while sleep 1s; do
    ../data/obftun \
        --dial "<server-ip>:8443" \
        --iface "obftun0" \
        --script "./ifconfig-client.sh" \
        --certificate "../data/client.crt" \
        --key "../data/client.key" \
        --ca "../data/ca.crt" \
        --verbose
done
