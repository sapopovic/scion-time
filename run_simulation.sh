#!/bin/bash

# Usage: ./run_simulation.sh [duration_in_seconds]

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 [duration_in_seconds]"
    exit 1
fi

DURATION="$1"

go build timeservice.go timeservice_t.go 
sleep 2

# --------------------------------------------------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/client_log_run1.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/mbg_run1.txt"

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "simulation_logs/$OUTPUT_FILE_TIMESERVICE"  2>&1
SERVICE_LOG_PID=$!

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!

echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

# --------------------------------------------------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/client_log_run2.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/mbg_run2.txt"

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "simulation_logs/$OUTPUT_FILE_TIMESERVICE"  2>&1
SERVICE_LOG_PID=$!

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!

echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

# --------------------------------------------------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

OUTPUT_FILE_TIMESERVICE="$SCRIPT_DIR/simulation_logs/client_log_run3.txt"
OUTPUT_FILE_MBG="$SCRIPT_DIR/simulation_logs/mbg_run3.txt"

sudo ./timeservice client -verbose -config testnet/client_sim.toml > "simulation_logs/$OUTPUT_FILE_TIMESERVICE"  2>&1
SERVICE_LOG_PID=$!

mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE_MBG" &
MBGSVCD_PID=$!

echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

kill "$MBGSVCD_PID"
kill "$SERVICE_LOG_PID"

sleep 1

# --------------------------------------------------------------------------------------------------------------------