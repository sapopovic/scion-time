cd ~
curl -LO https://github.com/pendulum-project/ntpd-rs/releases/download/v0.3.2/ntpd-rs_0.3.2-1jammy_amd64.deb
sudo dpkg -i ntpd-rs_0.3.2-1jammy_amd64.deb

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
