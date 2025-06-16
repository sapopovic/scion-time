#!/bin/bash

# Usage: ./measure_offset.sh [duration_in_seconds]


# Validate input
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 [duration_in_seconds]"
    exit 1
fi

DURATION="$1"

go build timeservice.go timeservice_t.go 
sleep 2

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Disable chrony, start scion-time
sudo systemctl stop chrony
sudo systemctl disable chrony
sudo systemctl enable timeservice-repo.service
sudo systemctl start timeservice-repo.service

journalctl -u timeservice-repo.service --since "now" --follow > "$SCRIPT_DIR/results/timeservice_logs.txt" &
SERVICE_LOG_PID=$!

# Output filename
OUTPUT_FILE="$SCRIPT_DIR/results/mbg_offsets.txt"

echo "Saving output to $OUTPUT_FILE"

# Start mbgsvcd in background and redirect output
mbgsvcd -f -Q -s 1 > "$OUTPUT_FILE" &
MBGSVCD_PID=$!

# Wait for specified duration
echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

# Stop mbgsvcd
echo "Stopping mbgsvcd (PID $MBGSVCD_PID)..."
kill "$MBGSVCD_PID"

kill "$SERVICE_LOG_PID"

# Wait a moment to ensure file flush
sleep 1

sudo systemctl stop timeservice-repo.service
sudo systemctl disable timeservice-repo.service
sudo systemctl enable chrony
sudo systemctl start chrony



