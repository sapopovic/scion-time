cd ~/scion-time

sudo killall timeservice
sudo killall timeservice
sudo killall timeservice

for c in 1 2 4 8 16 32 64 128 192 256 320 384;
do 
    for i in $(seq 1 $c)
    do 
        ./timeservice benchmark -config testnet/nts_benchmark.toml &
    done
    sleep 20
    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice
done

echo "Done"