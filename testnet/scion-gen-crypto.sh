#!/usr/bin/env bash
set -Eeuo pipefail

cd ~/scion

rm -rf gen*
export PYTHONPATH=python/:.
printf '#!/bin/bash\necho "0.0.0.0"' > tools/docker-ip
python3 python/topology/generator.py -c ~/scion-time/testnet/tiny4.topo
rm gen/jaeger-dc.yml
mkdir gen-cache

cd ~/scion-time/testnet/
rm -rf gen/ASff00_0_110/certs
rm -rf gen/ASff00_0_110/crypto
rm -rf gen/ASff00_0_110/keys
rm -rf gen/ASff00_0_111/certs
rm -rf gen/ASff00_0_111/crypto
rm -rf gen/ASff00_0_111/keys
rm -rf gen/ASff00_0_112/certs
rm -rf gen/ASff00_0_112/crypto
rm -rf gen/ASff00_0_112/keys
rm -rf gen/ISD1/trcs
rm -rf gen/certs
rm -rf gen/trcs
rm -rf gen-eh/ASff00_0_111/certs
rm -rf gen-eh/ASff00_0_111/crypto
rm -rf gen-eh/ASff00_0_111/keys
rm -rf gen-eh/ASff00_0_112/certs
rm -rf gen-eh/ASff00_0_112/crypto
rm -rf gen-eh/ASff00_0_112/keys

cp -r ~/scion/gen/ASff00_0_110/certs ~/scion-time/testnet/gen/ASff00_0_110/
cp -r ~/scion/gen/ASff00_0_110/crypto ~/scion-time/testnet/gen/ASff00_0_110/
cp -r ~/scion/gen/ASff00_0_110/keys ~/scion-time/testnet/gen/ASff00_0_110/

cp -r ~/scion/gen/ASff00_0_111/certs ~/scion-time/testnet/gen/ASff00_0_111/
cp -r ~/scion/gen/ASff00_0_111/crypto ~/scion-time/testnet/gen/ASff00_0_111/
cp -r ~/scion/gen/ASff00_0_111/keys ~/scion-time/testnet/gen/ASff00_0_111/

cp -r ~/scion/gen/ASff00_0_112/certs ~/scion-time/testnet/gen/ASff00_0_112/
cp -r ~/scion/gen/ASff00_0_112/crypto ~/scion-time/testnet/gen/ASff00_0_112/
cp -r ~/scion/gen/ASff00_0_112/keys ~/scion-time/testnet/gen/ASff00_0_112/

cp -r ~/scion/gen/ISD1/trcs ~/scion-time/testnet/gen/ISD1/
cp -r ~/scion/gen/certs ~/scion-time/testnet/gen/
cp -r ~/scion/gen/trcs ~/scion-time/testnet/gen/

cp -r ~/scion/gen/ASff00_0_111/certs ~/scion-time/testnet/gen-eh/ASff00_0_111/
cp -r ~/scion/gen/ASff00_0_111/crypto ~/scion-time/testnet/gen-eh/ASff00_0_111/
cp -r ~/scion/gen/ASff00_0_111/keys ~/scion-time/testnet/gen-eh/ASff00_0_111/

cp -r ~/scion/gen/ASff00_0_112/certs ~/scion-time/testnet/gen-eh/ASff00_0_112/
cp -r ~/scion/gen/ASff00_0_112/crypto ~/scion-time/testnet/gen-eh/ASff00_0_112/
cp -r ~/scion/gen/ASff00_0_112/keys ~/scion-time/testnet/gen-eh/ASff00_0_112/

rm -rf gen-cache
mkdir gen-cache
