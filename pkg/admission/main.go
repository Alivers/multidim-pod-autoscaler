package main

import "flag"

var (
	certsConfiguration = &certsConfig{
		clientCaFile:  flag.String("client-ca-file", "/etc/mpa-tls-certs/caCert.pem", "CA证书的路径"),
		tlsCertFile:   flag.String("tls-cert-file", "/etc/mpa-tls-certs/serverCert.pem", "server证书的路径"),
		tlsPrivateKey: flag.String("tls-private-key", "/etc/mpa-tls-certs/serverKey.pem", "server秘钥的路径"),
	}

	test = certsConfiguration
)
