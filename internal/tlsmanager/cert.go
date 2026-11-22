package tlsmanager

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gymbag/internal/host"
)

// CertificateError indicates a certificate provisioning or validation failure.
type CertificateError struct {
	Msg string
}

func (e *CertificateError) Error() string { return e.Msg }

func certErrorf(format string, args ...any) error {
	return &CertificateError{Msg: fmt.Sprintf(format, args...)}
}

// LoadLeafCertificate parses the first PEM CERTIFICATE block from pemBytes.
func LoadLeafCertificate(pemBytes []byte) (*x509.Certificate, error) {
	rest := pemBytes
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, certErrorf("invalid X.509 certificate: %v", err)
		}
		return cert, nil
	}
	return nil, certErrorf("certificate file does not contain a PEM certificate")
}

// CertificateNames extracts all DNS SANs, IP SANs, and Subject CommonName from cert.
func CertificateNames(cert *x509.Certificate) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(v string) {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			return
		}
		if _, exists := seen[v]; !exists {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	for _, dns := range cert.DNSNames {
		add(dns)
	}
	for _, ip := range cert.IPAddresses {
		add(ip.String())
	}
	if cert.Subject.CommonName != "" {
		add(cert.Subject.CommonName)
	}
	return out
}

// CertificateCovers reports whether cert covers every domain in domains (exact or single-label wildcard).
func CertificateCovers(cert *x509.Certificate, domains []string) bool {
	names := CertificateNames(cert)
	for _, domain := range domains {
		covered := false
		for _, pattern := range names {
			if host.Covers(pattern, domain) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// CertificateExpiresWithin reports whether the leaf certificate at certPath expires within days.
func CertificateExpiresWithin(certPath string, days int) (bool, error) {
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return true, err
	}
	cert, err := LoadLeafCertificate(raw)
	if err != nil {
		return true, err
	}
	remaining := time.Until(cert.NotAfter)
	return remaining <= time.Duration(days)*24*time.Hour, nil
}

// CertificateNeedsRenewal reports whether leaf is expired or within the renewal threshold,
// capping the threshold for short-lived certificates (<= 14 days, e.g. 160h IP certs) to 1/3 of lifetime.
func CertificateNeedsRenewal(leaf *x509.Certificate, withinDays int) bool {
	if leaf == nil {
		return true
	}
	if withinDays <= 0 {
		withinDays = 30
	}
	threshold := time.Duration(withinDays) * 24 * time.Hour
	lifetime := leaf.NotAfter.Sub(leaf.NotBefore)
	if lifetime > 0 && lifetime <= 14*24*time.Hour && threshold >= lifetime {
		third := lifetime / 3
		if lifetime >= 3*time.Hour && third < time.Hour {
			third = time.Hour
		}
		threshold = third
	}
	return time.Until(leaf.NotAfter) <= threshold
}

// BuildSelfSigned generates an ECDSA P-256 X.509v3 certificate and PKCS#8 private key covering domains.
func BuildSelfSigned(domains []string, days int) (certPEM, keyPEM []byte, err error) {
	if len(domains) == 0 {
		return nil, nil, certErrorf("a self-signed certificate needs at least one domain")
	}
	if days <= 0 {
		days = 90
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, certErrorf("failed to generate ECDSA key: %v", err)
	}

	var dnsNames []string
	var ipAddrs []net.IP
	for _, d := range domains {
		norm := host.Normalize(d)
		if ip := net.ParseIP(norm); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, norm)
		}
	}

	cn := host.Normalize(domains[0])
	if len(cn) > 64 {
		cn = cn[:64]
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, certErrorf("failed to generate serial number: %v", err)
	}

	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		Issuer:                pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(time.Duration(days) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, certErrorf("failed to sign certificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyPEM, err = MarshalPrivateKeyPEM(priv)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// MarshalPrivateKeyPEM encodes signer into an unencrypted PKCS#8 PRIVATE KEY PEM block.
func MarshalPrivateKeyPEM(signer crypto.Signer) ([]byte, error) {
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(signer)
	if err != nil {
		return nil, certErrorf("failed to marshal private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes}), nil
}

// ASN.1 OIDs for PKCS#8 PBES2 (RFC 5958 & RFC 8018).
var (
	oidPBES2          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 10}
	oidHMACWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}
	oidAES128CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
)

type encryptedPrivateKeyInfo struct {
	EncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedData       []byte
}

type pbes2Params struct {
	KeyDerivationFunc pkix.AlgorithmIdentifier
	EncryptionScheme  pkix.AlgorithmIdentifier
}

type pbkdf2Params struct {
	Salt           []byte
	IterationCount int
	KeyLength      int                      `asn1:"optional"`
	PRF            pkix.AlgorithmIdentifier `asn1:"optional"`
}

// EncryptPKCS8PrivateKeyPEM encrypts key using PKCS#8 PBES2 (PBKDF2-HMAC-SHA256 + AES-256-CBC) into PEM.
func EncryptPKCS8PrivateKeyPEM(key any, password string) ([]byte, error) {
	if password == "" {
		return nil, certErrorf("password must not be empty when encrypting a private key")
	}
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, certErrorf("failed to marshal PKCS#8 private key: %v", err)
	}

	salt := make([]byte, 16)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	const iterations = 60000
	derivedKey, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return nil, certErrorf("PBKDF2 key derivation failed: %v", err)
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, err
	}
	padLen := aes.BlockSize - (len(pkcs8DER) % aes.BlockSize)
	padded := make([]byte, len(pkcs8DER)+padLen)
	copy(padded, pkcs8DER)
	for i := len(pkcs8DER); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	kdfParamsDER, err := asn1.Marshal(pbkdf2Params{
		Salt:           salt,
		IterationCount: iterations,
		PRF: pkix.AlgorithmIdentifier{
			Algorithm:  oidHMACWithSHA256,
			Parameters: asn1.RawValue{Tag: asn1.TagNull},
		},
	})
	if err != nil {
		return nil, err
	}
	ivDER, err := asn1.Marshal(iv)
	if err != nil {
		return nil, err
	}
	pbes2DER, err := asn1.Marshal(pbes2Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{
			Algorithm:  oidPBKDF2,
			Parameters: asn1.RawValue{FullBytes: kdfParamsDER},
		},
		EncryptionScheme: pkix.AlgorithmIdentifier{
			Algorithm:  oidAES256CBC,
			Parameters: asn1.RawValue{FullBytes: ivDER},
		},
	})
	if err != nil {
		return nil, err
	}
	epkiDER, err := asn1.Marshal(encryptedPrivateKeyInfo{
		EncryptionAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm:  oidPBES2,
			Parameters: asn1.RawValue{FullBytes: pbes2DER},
		},
		EncryptedData: ciphertext,
	})
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: epkiDER}), nil
}

func decryptPKCS8EncryptedPrivateKey(der []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, certErrorf("private key is encrypted; supply a key password (GYMBAG_KEY_PASSWORD)")
	}
	var epki encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(der, &epki); err != nil {
		return nil, certErrorf("invalid PKCS#8 encrypted private key: %v", err)
	}
	if !epki.EncryptionAlgorithm.Algorithm.Equal(oidPBES2) {
		return nil, certErrorf("unsupported PKCS#8 encryption algorithm %s", epki.EncryptionAlgorithm.Algorithm)
	}
	var params pbes2Params
	if _, err := asn1.Unmarshal(epki.EncryptionAlgorithm.Parameters.FullBytes, &params); err != nil {
		return nil, certErrorf("invalid PBES2 parameters: %v", err)
	}
	if !params.KeyDerivationFunc.Algorithm.Equal(oidPBKDF2) {
		return nil, certErrorf("unsupported KDF algorithm %s", params.KeyDerivationFunc.Algorithm)
	}
	var kdf pbkdf2Params
	if _, err := asn1.Unmarshal(params.KeyDerivationFunc.Parameters.FullBytes, &kdf); err != nil {
		return nil, certErrorf("invalid PBKDF2 parameters: %v", err)
	}
	if kdf.IterationCount <= 0 || len(kdf.Salt) == 0 {
		return nil, certErrorf("invalid PBKDF2 salt or iteration count")
	}

	var keyLen int
	switch {
	case params.EncryptionScheme.Algorithm.Equal(oidAES128CBC):
		keyLen = 16
	case params.EncryptionScheme.Algorithm.Equal(oidAES192CBC):
		keyLen = 24
	case params.EncryptionScheme.Algorithm.Equal(oidAES256CBC):
		keyLen = 32
	default:
		return nil, certErrorf("unsupported PBES2 cipher %s", params.EncryptionScheme.Algorithm)
	}

	var iv []byte
	if _, err := asn1.Unmarshal(params.EncryptionScheme.Parameters.FullBytes, &iv); err != nil || len(iv) != aes.BlockSize {
		return nil, certErrorf("invalid AES-CBC initialization vector")
	}

	var hashFn func() hash.Hash
	switch {
	case len(kdf.PRF.Algorithm) == 0 || kdf.PRF.Algorithm.Equal(oidHMACWithSHA1):
		hashFn = sha1.New
	case kdf.PRF.Algorithm.Equal(oidHMACWithSHA256):
		hashFn = sha256.New
	case kdf.PRF.Algorithm.Equal(oidHMACWithSHA384):
		hashFn = sha512.New384
	case kdf.PRF.Algorithm.Equal(oidHMACWithSHA512):
		hashFn = sha512.New
	default:
		return nil, certErrorf("unsupported PBKDF2 PRF %s", kdf.PRF.Algorithm)
	}

	if len(epki.EncryptedData) == 0 || len(epki.EncryptedData)%aes.BlockSize != 0 {
		return nil, certErrorf("encrypted private key ciphertext is not block-aligned")
	}

	derivedKey, err := pbkdf2.Key(hashFn, password, kdf.Salt, kdf.IterationCount, keyLen)
	if err != nil {
		return nil, certErrorf("PBKDF2 key derivation failed: %v", err)
	}
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(epki.EncryptedData))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, epki.EncryptedData)
	return stripPKCS7Padding(plain, aes.BlockSize)
}

func isRFC1423EncryptedBlock(b *pem.Block) bool {
	if strings.Contains(strings.ToUpper(b.Headers["Proc-Type"]), "ENCRYPTED") {
		return true
	}
	return b.Headers["DEK-Info"] != ""
}

func decryptRFC1423PEMBlock(b *pem.Block, password string) ([]byte, error) {
	if password == "" {
		return nil, certErrorf("private key is encrypted; supply a key password (GYMBAG_KEY_PASSWORD)")
	}
	dekInfo := strings.TrimSpace(b.Headers["DEK-Info"])
	cipherName, ivHex, ok := strings.Cut(dekInfo, ",")
	if !ok {
		return nil, certErrorf("malformed DEK-Info header in encrypted PEM block")
	}
	iv, err := hex.DecodeString(strings.TrimSpace(ivHex))
	if err != nil || len(iv) < 8 {
		return nil, certErrorf("invalid IV in encrypted PEM DEK-Info header")
	}

	var keyLen int
	var newBlock func([]byte) (cipher.Block, error)
	switch strings.ToUpper(strings.TrimSpace(cipherName)) {
	case "AES-128-CBC":
		keyLen = 16
		newBlock = aes.NewCipher
	case "AES-192-CBC":
		keyLen = 24
		newBlock = aes.NewCipher
	case "AES-256-CBC":
		keyLen = 32
		newBlock = aes.NewCipher
	case "DES-EDE3-CBC":
		keyLen = 24
		newBlock = des.NewTripleDESCipher
	default:
		return nil, certErrorf("unsupported PEM cipher %q", cipherName)
	}

	derivedKey := evpBytesToKeyMD5([]byte(password), iv[:8], keyLen)
	ciph, err := newBlock(derivedKey)
	if err != nil {
		return nil, err
	}
	if len(iv) != ciph.BlockSize() || len(b.Bytes) == 0 || len(b.Bytes)%ciph.BlockSize() != 0 {
		return nil, certErrorf("invalid encrypted PEM block size")
	}
	plain := make([]byte, len(b.Bytes))
	cipher.NewCBCDecrypter(ciph, iv).CryptBlocks(plain, b.Bytes)
	return stripPKCS7Padding(plain, ciph.BlockSize())
}

func evpBytesToKeyMD5(password, salt []byte, keyLen int) []byte {
	var derived []byte
	var prev []byte
	for len(derived) < keyLen {
		h := md5.New()
		h.Write(prev)
		h.Write(password)
		h.Write(salt)
		prev = h.Sum(nil)
		derived = append(derived, prev...)
	}
	return derived[:keyLen]
}

func stripPKCS7Padding(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, certErrorf("failed to decrypt private key (incorrect password or corrupt data)")
	}
	padLen := int(data[len(data)-1])
	if padLen < 1 || padLen > blockSize || padLen > len(data) {
		return nil, certErrorf("failed to decrypt private key (incorrect password)")
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, certErrorf("failed to decrypt private key (incorrect password)")
		}
	}
	return data[:len(data)-padLen], nil
}

func parseUnencryptedPrivateKeyDER(der []byte) (crypto.Signer, error) {
	if parsed, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return validateCryptoSigner(parsed)
	}
	if ecKey, err := x509.ParseECPrivateKey(der); err == nil {
		return validateCryptoSigner(ecKey)
	}
	if rsaKey, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return validateCryptoSigner(rsaKey)
	}
	return nil, certErrorf("failed to parse private key (incorrect password or unsupported key format)")
}

func validateCryptoSigner(raw any) (crypto.Signer, error) {
	switch k := raw.(type) {
	case *ecdsa.PrivateKey:
		return k, nil
	case *rsa.PrivateKey:
		if k.N == nil || k.N.BitLen() < 2048 {
			return nil, certErrorf("RSA private key must be at least 2048 bits")
		}
		if err := k.Validate(); err != nil {
			return nil, certErrorf("invalid RSA private key: %v", err)
		}
		return k, nil
	case ed25519.PrivateKey:
		return k, nil
	default:
		return nil, certErrorf("unsupported private key type %T", raw)
	}
}

// ParsePrivateKeyPEM parses an unencrypted or password-protected PEM private key (PKCS#8, EC, RSA).
func ParsePrivateKeyPEM(keyPEM []byte, password string) (crypto.Signer, error) {
	rest := keyPEM
	var sawBlock bool
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "EC PARAMETERS" || block.Type == "CERTIFICATE" {
			continue
		}
		sawBlock = true

		if block.Type == "ENCRYPTED PRIVATE KEY" {
			der, err := decryptPKCS8EncryptedPrivateKey(block.Bytes, password)
			if err != nil {
				return nil, err
			}
			return parseUnencryptedPrivateKeyDER(der)
		}

		if isRFC1423EncryptedBlock(block) {
			der, err := decryptRFC1423PEMBlock(block, password)
			if err != nil {
				return nil, err
			}
			return parseUnencryptedPrivateKeyDER(der)
		}

		return parseUnencryptedPrivateKeyDER(block.Bytes)
	}
	if !sawBlock {
		return nil, certErrorf("private key file does not contain a PEM private key")
	}
	return nil, certErrorf("private key could not be parsed")
}

// LoadKeyPair reads an unencrypted PEM certificate chain and PEM private key, verifying public/private key match.
func LoadKeyPair(certPath, keyPath string) (tls.Certificate, *x509.Certificate, error) {
	return LoadKeyPairWithPassword(certPath, keyPath, "")
}

// LoadKeyPairWithPassword reads a PEM certificate chain and an unencrypted or password-protected PEM private key.
func LoadKeyPairWithPassword(certPath, keyPath, password string) (tls.Certificate, *x509.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	leaf, err := LoadLeafCertificate(certPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	signer, err := ParsePrivateKeyPEM(keyPEM, password)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	unencryptedKeyPEM, err := MarshalPrivateKeyPEM(signer)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, unencryptedKeyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	pair.Leaf = leaf
	return pair, leaf, nil
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func writePublicFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// HSTSEnabled reports whether Strict-Transport-Security should be enabled.
func HSTSEnabled(explicit *bool, source string) bool {
	if explicit != nil {
		return *explicit
	}
	return source == "acme"
}

// IsCertificateError reports whether err is or wraps a *CertificateError.
func IsCertificateError(err error) bool {
	var ce *CertificateError
	return errors.As(err, &ce)
}
