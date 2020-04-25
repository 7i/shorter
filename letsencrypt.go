package main

import (
	"crypto/rand"
	"crypto/tls"
	"net/http"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// getAuroCertTLSConf is used if UseLetsEncrypt is set to true.
// Note that a CertDir must be specified in the config if UseLetsEncrypt is set to true
func getServer(mux *http.ServeMux) (server *http.Server) {
	m := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(config.CertDir),
		HostPolicy: autocert.HostWhitelist(config.DomainName),
		Email:      config.Email,
	}
	tlsConf := &tls.Config{
		Rand:                     rand.Reader,
		Time:                     time.Now,
		NextProtos:               []string{acme.ALPNProto, "http/1.1"}, // add http2.NextProtoTLS?
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		GetCertificate:           m.GetCertificate,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
	}
	server = &http.Server{
		Addr:      config.TLSAddressPort,
		Handler:   mux,
		TLSConfig: tlsConf,
		// https://blog.bracebin.com/achieving-perfect-ssl-labs-score-with-go
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}
	// Handle ACME "http-01" challenge responses on external port 80.
	go http.ListenAndServe(config.AddressPort, m.HTTPHandler(nil))
	return
}
