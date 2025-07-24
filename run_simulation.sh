go build timeservice.go  timeservice_t.go

sleep 1

sudo ./timeservice client -verbose -config testnet/client_sim.toml > simulation_logs/client_log.txt  2>&1