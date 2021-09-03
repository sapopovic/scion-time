#!/usr/bin/env bash
set -Eeuo pipefail

# See https://yakking.branchable.com/posts/networking-4-namespaces-and-multi-host-routing/

sudo ip netns add near-0
sudo ip netns add far-0

sudo ip netns add near-1
sudo ip netns add far-1

sudo ip link add one address 00:76:65:74:68:31 type veth peer name two address 00:76:65:74:68:32
sudo ip link add three address 00:76:65:74:68:33 type veth peer name four address 00:76:65:74:68:34

sudo ip link add five address 00:76:65:74:68:35 type veth peer name six address 00:76:65:74:68:36
sudo ip link add seven address 00:76:65:74:68:37 type veth peer name eight address 00:76:65:74:68:38

sudo ip link add infra1 address 00:56:45:54:48:31 type veth peer name infra2 address 00:56:45:54:48:32
sudo ip link add infra5 address 00:56:45:54:48:35 type veth peer name infra6 address 00:56:45:54:48:36

sudo ip link set dev two netns near-0
sudo ip link set dev three netns near-0
sudo ip link set dev four netns far-0

sudo ip link set dev infra2 netns near-0

sudo ip link set dev six netns near-1
sudo ip link set dev seven netns near-1
sudo ip link set dev eight netns far-1

sudo ip link set dev infra6 netns near-1

sudo ip address add 10.248.1.1/24 dev one
sudo ip netns exec near-0 ip address add 10.248.1.2/24 dev two
sudo ip netns exec near-0 ip address add 10.248.2.1/24 dev three
sudo ip netns exec far-0 ip address add 10.248.2.2/24 dev four
sudo ip netns exec far-0 ip address add 10.248.3.1/24 dev lo

sudo ip address add 10.248.7.1/24 dev infra1
sudo ip netns exec near-0 ip address add 10.248.7.2/24 dev infra2

sudo ip address add 10.248.4.1/24 dev five
sudo ip netns exec near-1 ip address add 10.248.4.2/24 dev six
sudo ip netns exec near-1 ip address add 10.248.5.1/24 dev seven
sudo ip netns exec far-1 ip address add 10.248.5.2/24 dev eight
sudo ip netns exec far-1 ip address add 10.248.6.1/24 dev lo

sudo ip address add 10.248.8.1/24 dev infra5
sudo ip netns exec near-1 ip address add 10.248.8.2/24 dev infra6

sudo ip link set dev one up
sudo ip netns exec near-0 ip link set dev two up
sudo ip netns exec near-0 ip link set dev three up
sudo ip netns exec far-0 ip link set dev four up
sudo ip netns exec far-0 ip link set dev lo up

sudo ip link set dev infra1 up
sudo ip netns exec near-0 ip link set dev infra2 up

sudo ip link set dev five up
sudo ip netns exec near-1 ip link set dev six up
sudo ip netns exec near-1 ip link set dev seven up
sudo ip netns exec far-1 ip link set dev eight up
sudo ip netns exec far-1 ip link set dev lo up

sudo ip link set dev infra5 up
sudo ip netns exec near-1 ip link set dev infra6 up

sudo ip route add 10.248.2.0/24 dev one via 10.248.1.2
sudo ip route add 10.248.3.0/24 dev one via 10.248.1.2

sudo ip route add 10.248.5.0/24 dev five via 10.248.4.2
sudo ip route add 10.248.6.0/24 dev five via 10.248.4.2

sudo ip netns exec near-0 ip route add 10.248.4.0/24 dev two via 10.248.1.1
sudo ip netns exec near-0 ip route add 10.248.5.0/24 dev two via 10.248.1.1
sudo ip netns exec near-0 ip route add 10.248.6.0/24 dev two via 10.248.1.1

sudo ip netns exec near-0 ip route add 10.248.3.0/24 dev three via 10.248.2.2

sudo ip netns exec near-0 ip route change 10.248.1.0/24 dev two via 10.248.1.1
sudo ip netns exec near-0 ip route change 10.248.2.0/24 dev three via 10.248.2.2

sudo ip netns exec far-0 ip route add 10.248.1.0/24 dev four via 10.248.2.1
sudo ip netns exec far-0 ip route add 10.248.4.0/24 dev four via 10.248.2.1
sudo ip netns exec far-0 ip route add 10.248.5.0/24 dev four via 10.248.2.1
sudo ip netns exec far-0 ip route add 10.248.6.0/24 dev four via 10.248.2.1

sudo ip netns exec near-1 ip route add 10.248.1.0/24 dev six via 10.248.4.1
sudo ip netns exec near-1 ip route add 10.248.2.0/24 dev six via 10.248.4.1
sudo ip netns exec near-1 ip route add 10.248.3.0/24 dev six via 10.248.4.1

sudo ip netns exec near-1 ip route add 10.248.6.0/24 dev seven via 10.248.5.2

sudo ip netns exec near-1 ip route change 10.248.4.0/24 dev six via 10.248.4.1
sudo ip netns exec near-1 ip route change 10.248.5.0/24 dev seven via 10.248.5.2

sudo ip netns exec far-1 ip route add 10.248.1.0/24 dev eight via 10.248.5.1
sudo ip netns exec far-1 ip route add 10.248.2.0/24 dev eight via 10.248.5.1
sudo ip netns exec far-1 ip route add 10.248.3.0/24 dev eight via 10.248.5.1
sudo ip netns exec far-1 ip route add 10.248.4.0/24 dev eight via 10.248.5.1
