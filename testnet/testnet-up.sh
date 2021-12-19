#!/usr/bin/env bash
set -Eeuo pipefail

sudo ip a add 10.10.0.0/16 dev lo

sudo ip netns add netns0
sudo ip netns add netns1

sudo ip link add veth0 address 00:76:65:74:68:30 type veth peer name veth1 address 00:76:65:74:68:31
sudo ip link add veth2 address 00:76:65:74:68:32 type veth peer name veth3 address 00:76:65:74:68:33

sudo ip link set dev veth1 netns netns0
sudo ip link set dev veth3 netns netns1

sudo ip link set veth0 up
sudo ip link set veth2 up

sudo ip netns exec netns0 ip link set lo up
sudo ip netns exec netns0 ip link set veth1 up

sudo ip netns exec netns1 ip link set lo up
sudo ip netns exec netns1 ip link set veth3 up

sudo ip netns exec netns0 ip address add 10.0.0.10/24 dev veth1

sudo ip netns exec netns1 ip address add 10.0.0.20/24 dev veth3

sudo ip link add br0 type bridge
sudo ip link set br0 up

sudo ip link set veth0 master br0
sudo ip link set veth2 master br0

sudo ip address add 10.0.0.1/24 dev br0

sudo ip netns exec netns0 ip route add default via 10.0.0.1
sudo ip netns exec netns1 ip route add default via 10.0.0.1

sudo sysctl -w net.ipv4.ip_forward=1

sudo iptables -P FORWARD ACCEPT
sudo iptables -t nat -A POSTROUTING -s 10.0.0.1/24 ! -o br0 -j MASQUERADE

sudo mkdir -p /etc/netns/netns0
sudo bash -c "echo 'nameserver 9.9.9.9' > /etc/netns/netns0/resolv.conf"
sudo mkdir -p /etc/netns/netns1
sudo bash -c "echo 'nameserver 9.9.9.9' > /etc/netns/netns1/resolv.conf"
