#!/bin/bash

cd $(dirname $0)

while sleep 1s; do
    ../data/obftun \
        --bind ":8443" \
        --iface "obftun0" \
        --script "./ifconfig-server.sh" \
        --certificate "../data/server.crt" \
        --key "../data/server.key" \
        --ca "../data/ca.crt" \
        --verbose # Remove "verbose" on PROD
done
