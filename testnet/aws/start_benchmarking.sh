# This script will create requests with different versions
# 1. old version nonauth
# 2. old version nts
# 3. gopacket nonauth
# 4. gopacket nts



# install go
cd ~

sudo rm -rf /usr/local/go
curl -LO https://golang.org/dl/go1.19.7.linux-arm64.tar.gz
echo "071ea7bf386fdd08df524859b878d99fc359e491e7ad65c1c1cc55b67972c882 go1.19.7.linux-arm64.tar.gz" | sha256sum -c
sudo tar -C /usr/local -xzf go1.19.7.linux-arm64.tar.gz
rm go1.19.7.linux-arm64.tar.gz
export PATH=$PATH:/usr/local/go/bin
go version

# build timeservice
cd ~

sudo rm -r scion-time

git clone -b aws https://github.com/aaronbojarski/scion-time.git

cd ~/scion-time
go build timeservice.go timeservicex.go
openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out testnet/gen/tls.crt -keyout testnet/gen/tls.key -config testnet/tls-cert.conf



cd ~/scion-time

sudo killall timeservice
sudo killall timeservice
sudo killall timeservice

for c in 1 2 4 8 16 32 64 128 192 256 320 384;
do 
    for i in $(seq 1 $c)
    do 
        ./timeservice benchmark -config ~/nonauth_benchmark.toml &
    done
    sleep 20
    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice
    sleep 5
done

echo "Done 1"


sudo killall timeservice
sudo killall timeservice
sudo killall timeservice

for c in 1 2 4 8 16 32 64 128 192 256 320 384;
do 
    for i in $(seq 1 $c)
    do 
        ./timeservice benchmark -config ~/nts_benchmark.toml &
    done
    sleep 20
    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice
    sleep 5
done

echo "Done 2"


# build timeservice
cd ~

sudo rm -r scion-time

git clone -b gopacket https://github.com/aaronbojarski/scion-time.git

cd ~/scion-time
go build timeservice.go timeservicex.go
openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out testnet/gen/tls.crt -keyout testnet/gen/tls.key -config testnet/tls-cert.conf



cd ~/scion-time

sudo killall timeservice
sudo killall timeservice
sudo killall timeservice

for c in 1 2 4 8 16 32 64 128 192 256 320 384;
do 
    for i in $(seq 1 $c)
    do 
        ./timeservice benchmark -config ~/nonauth_benchmark.toml &
    done
    sleep 20
    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice
    sleep 5
done

echo "Done 3"


sudo killall timeservice
sudo killall timeservice
sudo killall timeservice

for c in 1 2 4 8 16 32 64 128 192 256 320 384;
do 
    for i in $(seq 1 $c)
    do 
        ./timeservice benchmark -config ~/nts_benchmark.toml &
    done
    sleep 20
    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice
    sleep 5
done

echo "Done 4"