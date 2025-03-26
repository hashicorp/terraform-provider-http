package provider

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func generateCert(t *testing.T) (string, string) {
	// Generate a new ECDSA private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "failed to generate private key")

	// Create certificate template
	certTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"My Organization"},
			CommonName:   "localhost",
		},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 1 year validity

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &priv.PublicKey, priv)
	require.NoError(t, err, "failed to create certificate")

	// Filenames for the certificate and key
	tmpDir := t.TempDir()
	cert := filepath.Join(tmpDir, "cert.pem")
	key := filepath.Join(tmpDir, "key.pem")

	// Save the certificate to a file
	certOut, err := os.Create(cert)
	require.NoError(t, err, "failed to open cert.pem for writing")
	defer certOut.Close()
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, err, "failed to write data to cert.pem")

	// Save the private key to a file
	keyOut, err := os.Create(key)
	require.NoError(t, err, "failed to open key.pem for writing")
	defer keyOut.Close()
	privBytes, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err, "failed to marshal private key")
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	require.NoError(t, err, "failed to write data to key.pem")

	return cert, key
}
