package cluster

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var ErrInvalidKubeconfig = errors.New("invalid kubeconfig")

type kubeconfigDocument struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
			TLSServerName            string `yaml:"tls-server-name"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token                 string `yaml:"token"`
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
		} `yaml:"user"`
	} `yaml:"users"`
}

type ClientConfig struct {
	Server    string
	Transport *http.Transport
	Token     string
}

func ParseKubeconfig(raw []byte) (ClientConfig, error) {
	var document kubeconfigDocument
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return ClientConfig{}, fmt.Errorf("%w: malformed YAML", ErrInvalidKubeconfig)
	}
	if document.CurrentContext == "" {
		return ClientConfig{}, fmt.Errorf("%w: current-context is required", ErrInvalidKubeconfig)
	}
	var clusterName, userName string
	for _, item := range document.Contexts {
		if item.Name == document.CurrentContext {
			clusterName, userName = item.Context.Cluster, item.Context.User
			break
		}
	}
	if clusterName == "" {
		return ClientConfig{}, fmt.Errorf("%w: current context does not reference a cluster", ErrInvalidKubeconfig)
	}
	var server, caData, tlsServerName string
	var insecure bool
	for _, item := range document.Clusters {
		if item.Name == clusterName {
			server = item.Cluster.Server
			caData = item.Cluster.CertificateAuthorityData
			insecure = item.Cluster.InsecureSkipTLSVerify
			tlsServerName = strings.TrimSpace(item.Cluster.TLSServerName)
			break
		}
	}
	parsedURL, err := url.Parse(server)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return ClientConfig{}, fmt.Errorf("%w: API server must be an absolute HTTPS URL", ErrInvalidKubeconfig)
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure, ServerName: tlsServerName} //nolint:gosec -- explicitly requested by kubeconfig
	if caData != "" {
		decoded, err := base64.StdEncoding.DecodeString(caData)
		if err != nil {
			return ClientConfig{}, fmt.Errorf("%w: invalid certificate-authority-data", ErrInvalidKubeconfig)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(decoded) {
			return ClientConfig{}, fmt.Errorf("%w: certificate-authority-data contains no certificate", ErrInvalidKubeconfig)
		}
		tlsConfig.RootCAs = pool
	}
	var token string
	for _, item := range document.Users {
		if item.Name != userName {
			continue
		}
		token = strings.TrimSpace(item.User.Token)
		certData, keyData := item.User.ClientCertificateData, item.User.ClientKeyData
		if certData != "" || keyData != "" {
			certPEM, certErr := base64.StdEncoding.DecodeString(certData)
			keyPEM, keyErr := base64.StdEncoding.DecodeString(keyData)
			if certErr != nil || keyErr != nil {
				return ClientConfig{}, fmt.Errorf("%w: invalid client certificate data", ErrInvalidKubeconfig)
			}
			certificate, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return ClientConfig{}, fmt.Errorf("%w: invalid client certificate pair", ErrInvalidKubeconfig)
			}
			tlsConfig.Certificates = []tls.Certificate{certificate}
		}
		break
	}
	if token == "" && len(tlsConfig.Certificates) == 0 {
		return ClientConfig{}, fmt.Errorf("%w: token or client certificate is required", ErrInvalidKubeconfig)
	}
	return ClientConfig{Server: strings.TrimRight(server, "/"), Token: token, Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 5 * time.Second,
	}}, nil
}
