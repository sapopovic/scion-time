#!/bin/bash

# Usage: ./measure_offset.sh [duration_in_seconds]

# Validate input
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 [duration_in_seconds]"
    exit 1
fi

DURATION="$1"



### Chrony

# Disable scion-time, start chrony
sudo systemctl stop scion-timeservice-client.service
sudo systemctl disable scion-timeservice-client.service
sudo systemctl enable chrony
sudo systemctl start chrony

# Output filename
OUTPUT_FILE="offset_chrony.txt"

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

# Wait a moment to ensure file flush
sleep 1

# Plot results
echo "Generating plot with ./offplot $OUTPUT_FILE ..."
./offplot "$OUTPUT_FILE"

echo "Done. Output plot: ${OUTPUT_FILE%.txt}.pdf"




### SCION

GIT_REVISION=$(/home/ubuntu/scion-time/timeservice info | grep 'vcs.revision' | awk -F= '{print $2}')

# Disable chrony, start scion-time
sudo systemctl stop chrony
sudo systemctl disable chrony
sudo systemctl enable scion-timeservice-client.service
sudo systemctl start scion-timeservice-client.service

# Output filename
OUTPUT_FILE="offset_scion.txt"

echo "Saving output to $OUTPUT_FILE"

echo "GIT_REVISION=\"$GIT_REVISION\"" > "$OUTPUT_FILE"

# Start mbgsvcd in background and redirect output
mbgsvcd -f -Q -s 1 >> "$OUTPUT_FILE" &
MBGSVCD_PID=$!

# Wait for specified duration
echo "Waiting $DURATION seconds for measurements..."
sleep "$DURATION"

# Stop mbgsvcd
echo "Stopping mbgsvcd (PID $MBGSVCD_PID)..."
kill "$MBGSVCD_PID"

# Wait a moment to ensure file flush
sleep 1

# Plot results
echo "Generating plot with ./offplot $OUTPUT_FILE ..."
./offplot "$OUTPUT_FILE"

echo "Done. Output plot: ${OUTPUT_FILE%.txt}.pdf"


# to copy to local machine: 
# scp -r ubuntu@192.33.93.151:/home/ubuntu/repositories/scion-time/testnet/offplot .