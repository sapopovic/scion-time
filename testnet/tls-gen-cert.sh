openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out gen/tls.crt -keyout gen/tls.key -config tls-cert.conf
