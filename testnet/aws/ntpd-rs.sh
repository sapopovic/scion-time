cd ~
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

cargo install ntpd

sudo systemctl stop chronyd
sudo systemctl stop ntpd-rs

echo "[ req ]
distinguished_name = dn
prompt = no

[ dn ]
C = CH
ST = Zurich
L = .
O = .
OU = .
CN = ." > tls-cert.conf

openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out tls.crt -keyout tls.key -config tls-cert.conf


# sudo nano /etc/ntpd-rs/ntp.toml
# sudo systemctl edit ntpd-rs.service
# sudo systemctl daemon-reload
# sudo systemctl start ntpd-rs
