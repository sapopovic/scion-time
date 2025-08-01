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

# Goal: show influence of initial clock skew on asymmetry estimation (convergence). We use exp1 path set, same as exp4


# --------------------------------------------------------------------------------------------------------------------

# sudo systemctl stop chrony
# sudo systemctl disable chrony
# sudo systemctl start mbgsvcd.service
# 
# # 1b) Start with a small clock skew, then run for 30 minutes
# # chose bad "random" paths for warmup
# 
# OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/exp1_log_run1.txt"
# OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/exp1_mbg_run1.txt"
# 
# sudo ./timeservice client -verbose -config testnet/client_sim.toml > "$OUTPUT_FILE_TIMESERVICE"  2>&1 &
# SERVICE_LOG_PID=$!
# 
# mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
# MBGSVCD_PID=$!
# 
# #sleep 1 # the first entry in mbgsvcd file contains the offset at which we start
# echo "Waiting $DURATION seconds for measurements..."
# sleep "$DURATION"
# 
# kill "$MBGSVCD_PID"
# kill "$SERVICE_LOG_PID"
# 
# sleep 1
# 
# sudo systemctl enable chrony
# sudo systemctl start chrony
# sudo systemctl stop mbgsvcd.service

# --------------------------------------------------------------------------------------------------------------------


#sudo systemctl stop chrony
#sudo systemctl disable chrony
sudo systemctl start mbgsvcd.service

#sleep 600 # 10 minutes of drifting

# 1a) Start with a large clock skew = 1ms: disable chrony, wait 10 minutes (clock drifts), then start clockwire. compare to 1b) which paths were chosen and what the offset error is.
# Real delays (real asymmetry, not derived by client) can be found in the logs. Extract them for each path in S and there you know which paths would have been best.

OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/exp1_log_runA.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/exp1_mbg_runA.txt"

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!
sleep 2 # the first two entries in mbgsvcd file contain the offset at which we start, how big the clock skew is

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "$OUTPUT_FILE_TIMESERVICE"  2>&1 &
SERVICE_LOG_PID=$!

echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

sudo systemctl enable chrony
sudo systemctl start chrony
sudo systemctl stop mbgsvcd.service

# --------------------------------------------------------------------------------------------------------------------

# 1c) Wait till offset is big again (as in 1a). Then run without a warm up! Investigate how the curve converges.

# sleep 60 # let chrony correct
# 
# sudo systemctl stop chrony
# sudo systemctl disable chrony
# sudo systemctl start mbgsvcd.service
# 
# sleep 600
# 
# OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/exp1_log_run3.txt"
# OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/exp1_mbg_run3.txt"
# 
# sudo ./timeservice client -verbose -config testnet/client_sim.toml > "$OUTPUT_FILE_TIMESERVICE"  2>&1 &
# SERVICE_LOG_PID=$!
# 
# mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
# MBGSVCD_PID=$!
# 
# #sleep 1 # the first entry in mbgsvcd file contains the offset at which we start
# 
# echo "Waiting $DURATION seconds for measurements..."
# sleep "$DURATION"
# 
# kill "$MBGSVCD_PID"
# kill "$SERVICE_LOG_PID"
# 
# sleep 1
# 
# sudo systemctl enable chrony
# sudo systemctl start chrony
# sudo systemctl stop mbgsvcd.service

# --------------------------------------------------------------------------------------------------------------------