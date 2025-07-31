#!/bin/bash

# Usage: ./run_simulation.sh [duration_in_seconds]

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 [duration_in_seconds]"
    exit 1
fi

DURATION="$1"

go build timeservice.go timeservice_t.go 
sleep 2

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"


# export SCION_BIN=~/scion/bin
# cd ~/scion-time/testnet
# 
# rm -rf logs
# ./scion-topo-gen-crypto.sh
# ./testnet-up.sh
# 
# ./supervisor/supervisor.sh reload && sleep 1
# ./supervisor/supervisor.sh start all

# Goal: Simulation is reliable and reproducible. Clock drift is a source of undeterminism. The graphs are influenced by the initial local clock offset state.
# v6

sudo systemctl stop chrony
sudo systemctl disable chrony

sudo systemctl start mbgsvcd.service

# --------------------------------------------------------------------------------------------------------------------



OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/exp4_log_run1.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/exp4_mbg_run1.txt"

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "$OUTPUT_FILE_TIMESERVICE"  2>&1 &
SERVICE_LOG_PID=$!

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!

#sleep 1 # the first entry in mbgsvcd file contains the offset at which we start
echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

# --------------------------------------------------------------------------------------------------------------------

OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/exp4_log_run2.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/exp4_mbg_run2.txt"

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "$OUTPUT_FILE_TIMESERVICE"  2>&1 &
SERVICE_LOG_PID=$!

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!

#sleep 1 # the first entry in mbgsvcd file contains the offset at which we start

echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

# --------------------------------------------------------------------------------------------------------------------

OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/exp4_log_run3.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/exp4_mbg_run3.txt"

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "$OUTPUT_FILE_TIMESERVICE"  2>&1 &
SERVICE_LOG_PID=$!

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!

#sleep 1 # the first entry in mbgsvcd file contains the offset at which we start

echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

# --------------------------------------------------------------------------------------------------------------------

sudo systemctl enable chrony
sudo systemctl start chrony

sudo systemctl stop mbgsvcd.service