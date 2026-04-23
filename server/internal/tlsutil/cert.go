package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// EnsureCert returns paths to a TLS cert+key in dataDir, generating a
// self-signed pair on first run. The fingerprint is the SHA-256 of the DER
// certificate (clients pin this on first connect — TOFU).
func EnsureCert(dataDir string) (certPath, keyPath, fingerprint string, err error) {
	certPath = filepath.Join(dataDir, "tls.crt")
	keyPath = filepath.Join(dataDir, "tls.key")

	if _, e1 := os.Stat(certPath); e1 == nil {
		if _, e2 := os.Stat(keyPath); e2 == nil {
			fp, ferr := readFingerprint(certPath)
			return certPath, keyPath, fp, ferr
		}
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", "", err
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "cass-server"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost", "cass-server", "cass.local"},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", "", err
	}

	if err := writePEM(certPath, "CERTIFICATE", der); err != nil {
		return "", "", "", err
	}
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", "", err
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyBytes); err != nil {
		return "", "", "", err
	}

	sum := sha256.Sum256(der)
	return certPath, keyPath, formatFingerprint(sum), nil
}

func writePEM(path, blockType string, body []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: body})
}

func readFingerprint(certPath string) (string, error) {
	b, err := os.ReadFile(certPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return "", fmt.Errorf("no PEM data in %s", certPath)
	}
	sum := sha256.Sum256(block.Bytes)
	return formatFingerprint(sum), nil
}

func formatFingerprint(sum [32]byte) string {
	const hex = "0123456789ABCDEF"
	out := make([]byte, 0, len(sum)*3-1)
	for i, b := range sum {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[b>>4], hex[b&0x0f])
	}
	return string(out)
}
