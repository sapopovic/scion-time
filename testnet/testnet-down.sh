#!/usr/bin/env bash

# See https://yakking.branchable.com/posts/networking-4-namespaces-and-multi-host-routing/

set -Eeuo pipefail

sudo ip netns del far-0
sudo ip netns del near-0
sudo ip link del one

sudo ip link del infra1

sudo ip netns del far-1
sudo ip netns del near-1
sudo ip link del five

sudo ip link del infra5
