package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	certFileName = "cert.pem"
	keyFileName  = "key.pem"
)

// getCertsDir returns the path to the certificates directory.
func getCertsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "nfc-agent", "certs"), nil
}

// LoadOrGenerate loads existing TLS certificates or generates new self-signed ones.
// Returns a tls.Config ready for use with HTTPS server.
func LoadOrGenerate() (*tls.Config, error) {
	certsDir, err := getCertsDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get certs directory: %w", err)
	}

	certPath := filepath.Join(certsDir, certFileName)
	keyPath := filepath.Join(certsDir, keyFileName)

	// Check if certificates already exist
	if fileExists(certPath) && fileExists(keyPath) {
		// Load existing certificates
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			// If loading fails, regenerate
			return generateAndSave(certsDir, certPath, keyPath)
		}

		// Check if certificate is expired or will expire soon (within 30 days)
		if certNeedsRenewal(cert) {
			return generateAndSave(certsDir, certPath, keyPath)
		}

		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	// Generate new certificates
	return generateAndSave(certsDir, certPath, keyPath)
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// certNeedsRenewal checks if the certificate is expired or expires within 30 days.
func certNeedsRenewal(cert tls.Certificate) bool {
	if len(cert.Certificate) == 0 {
		return true
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return true
	}

	// Renew if expires within 30 days
	renewalThreshold := time.Now().Add(30 * 24 * time.Hour)
	return x509Cert.NotAfter.Before(renewalThreshold)
}

// generateAndSave generates a new self-signed certificate and saves it to disk.
func generateAndSave(certsDir, certPath, keyPath string) (*tls.Config, error) {
	// Ensure directory exists
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create certs directory: %w", err)
	}

	// Generate ECDSA private key (P-256 is secure and efficient)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Certificate valid for 1 year
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"NFC Agent"},
			CommonName:   "localhost",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		// Allow localhost access
		DNSNames: []string{"localhost"},
		// Also allow IP addresses
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	// Save certificate
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("failed to save certificate: %w", err)
	}

	// Save private key (restricted permissions)
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("failed to save private key: %w", err)
	}

	// Load the certificate we just saved
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load generated certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// GetCertPath returns the path to the certificate file (for display purposes).
func GetCertPath() string {
	certsDir, err := getCertsDir()
	if err != nil {
		return ""
	}
	return filepath.Join(certsDir, certFileName)
}
