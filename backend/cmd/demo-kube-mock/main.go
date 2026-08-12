// Command demo-kube-mock runs a minimal offline Kubernetes API simulation used
// by the M102 reproducible demo drill (scripts/demo-drill.sh).
//
// The mock serves the small Kubernetes API surface the platform demo journey
// touches: /version (probe), nodes, namespaces, pods, events, deployments,
// replicasets and metrics endpoints backed by deterministic fixtures. PATCH
// mutations (rollout restart / scale / image update / cordon) are merged into
// an in-memory store so the drill can verify a controlled action actually
// landed, and GET /mock/mutations returns the recorded mutations as evidence.
//
// The server always speaks HTTPS with a self-signed certificate generated at
// startup; the drill registers it with insecure-skip-tls-verify. It is a drill
// and development tool only, never a production Kubernetes API server.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:8443", "HTTPS listen address")
	health := flag.Bool("healthcheck", false, "probe liveness and exit 0 when healthy")
	flag.Parse()

	if *health {
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec -- local healthcheck only
		}}
		resp, err := client.Get("https://127.0.0.1:8443/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			os.Exit(0)
		}
		os.Exit(1)
	}

	certPEM, keyPEM, err := generateCert("aiops-demo-kube-mock")
	if err != nil {
		log.Fatalf("generate certificate: %v", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		log.Fatalf("load certificate: %v", err)
	}

	server := &http.Server{
		Addr:    *listen,
		Handler: newHandler(),
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		},
		ReadHeaderTimeout: 5 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		log.Printf("demo-kube-mock listening on https://%s", *listen)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()
	<-stop
	_ = server.Close()
}

// generateCert returns a self-signed certificate covering localhost, the
// compose service name and 127.0.0.1. The mock is registered with
// insecure-skip-tls-verify, so the SANs are informational.
func generateCert(cn string) (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost", "demo-mock"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, nil
}
