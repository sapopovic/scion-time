go build timeservice.go timeservice_t.go
LOG="logs.txt"

sudo systemctl stop chrony
sudo systemctl disable chrony
sudo systemctl enable scion-time-onepath.service
sudo systemctl start scion-time-onepath.service


journalctl -u scion-time-onepath.service --since "now" --follow > "$LOG" &
SERVICE_LOG_PID=$!

sleep 15

kill "$SERVICE_LOG_PID"

sudo systemctl stop scion-time-onepath.service
sudo systemctl disable scion-time-onepath.service
sudo systemctl enable chrony
sudo systemctl start chrony