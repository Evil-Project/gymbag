package tlsmanager

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gymbag/internal/host"
)

func b64url(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func b64urlDecode(raw string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(raw, "="))
}

func decodeEABHMACKey(raw string) ([]byte, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return nil, certErrorf("EAB HMAC key must not be empty")
	}
	unpadded := strings.TrimRight(cleaned, "=")
	if strings.ContainsAny(unpadded, "+/") {
		key, err := base64.RawStdEncoding.DecodeString(unpadded)
		if err != nil || len(key) == 0 {
			return nil, certErrorf("invalid base64 EAB HMAC key")
		}
		return key, nil
	}
	key, err := base64.RawURLEncoding.DecodeString(unpadded)
	if err != nil {
		if stdKey, stdErr := base64.RawStdEncoding.DecodeString(unpadded); stdErr == nil && len(stdKey) > 0 {
			return stdKey, nil
		}
		return nil, certErrorf("invalid base64url EAB HMAC key")
	}
	if len(key) == 0 {
		return nil, certErrorf("EAB HMAC key must not be empty")
	}
	return key, nil
}

func isValidACMEToken(token string) bool {
	if token == "" {
		return false
	}
	for i := 0; i < len(token); i++ {
		c := token[i]
		isAlphaNum := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !isAlphaNum {
			return false
		}
	}
	return true
}

// isPublicACMEIP reports whether raw is a globally routable IP address eligible for RFC 8738 ACME IP issuance.
func isPublicACMEIP(raw string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(host.Normalize(raw)))
	if err != nil || !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, p := range []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("2001:db8::/32"),
	} {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// isPublicACMEDomain reports whether raw is a publicly routable domain eligible for ACME HTTP-01 validation.
func isPublicACMEDomain(raw string) bool {
	if !host.IsPublic(raw) {
		return false
	}
	name := host.Normalize(raw)
	if !strings.Contains(name, ".") {
		return false
	}
	for _, reservedSuffix := range []string{".internal", ".test", ".invalid", ".home.arpa"} {
		if strings.HasSuffix(name, reservedSuffix) {
			return false
		}
	}
	return true
}

// isPublicACMEIdentifier reports whether raw is either a public ACME domain or (when allowIPs) a globally routable IP.
func isPublicACMEIdentifier(raw string, allowIPs bool) bool {
	if allowIPs && isPublicACMEIP(raw) {
		return true
	}
	return isPublicACMEDomain(raw)
}

// HTTP01Board stores in-flight ACME HTTP-01 challenge key authorizations served on port 80.
type HTTP01Board struct {
	mu     sync.RWMutex
	tokens map[string]string
}

// NewHTTP01Board creates an empty concurrency-safe HTTP01Board.
func NewHTTP01Board() *HTTP01Board {
	return &HTTP01Board{tokens: make(map[string]string)}
}

// Publish registers token -> authorization for HTTP-01 validation.
func (b *HTTP01Board) Publish(token, authorization string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens[token] = authorization
}

// Lookup retrieves the key authorization for token, returning ("", false) if invalid or absent.
func (b *HTTP01Board) Lookup(token string) (string, bool) {
	if !isValidACMEToken(token) {
		return "", false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.tokens[token]
	return val, ok
}

// Clear removes all published challenge tokens.
func (b *HTTP01Board) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	clear(b.tokens)
}

// ECJWK returns the RFC 7517 JWK representation of an ECDSA P-256 or P-384 public key.
func ECJWK(pub *ecdsa.PublicKey) map[string]string {
	crv := "P-256"
	coordLen := 32
	if pub.Curve == elliptic.P384() {
		crv = "P-384"
		coordLen = 48
	}
	xBytes := make([]byte, coordLen)
	yBytes := make([]byte, coordLen)
	pub.X.FillBytes(xBytes)
	pub.Y.FillBytes(yBytes)
	return map[string]string{
		"crv": crv,
		"kty": "EC",
		"x":   b64url(xBytes),
		"y":   b64url(yBytes),
	}
}

// PublicKeyJWK returns the RFC 7517 JWK representation of an ECDSA (P-256/P-384) or RSA public key.
func PublicKeyJWK(pub crypto.PublicKey) (map[string]string, error) {
	switch pk := pub.(type) {
	case *ecdsa.PublicKey:
		if pk == nil || pk.X == nil || pk.Y == nil {
			return nil, certErrorf("invalid ECDSA public key")
		}
		switch pk.Curve {
		case elliptic.P256(), elliptic.P384():
			return ECJWK(pk), nil
		default:
			return nil, certErrorf("ACME ECDSA account key must use P-256 or P-384 curve")
		}
	case *rsa.PublicKey:
		if pk == nil || pk.N == nil || pk.N.BitLen() < 2048 || pk.E <= 1 {
			return nil, certErrorf("ACME RSA account key must be at least 2048 bits")
		}
		return map[string]string{
			"e":   b64url(big.NewInt(int64(pk.E)).Bytes()),
			"kty": "RSA",
			"n":   b64url(pk.N.Bytes()),
		}, nil
	default:
		return nil, certErrorf("unsupported ACME account public key type %T", pub)
	}
}

// PublicKeyThumbprint computes the RFC 7638 SHA-256 thumbprint of pub.
func PublicKeyThumbprint(pub crypto.PublicKey) (string, error) {
	jwk, err := PublicKeyJWK(pub)
	if err != nil {
		return "", err
	}
	// encoding/json sorts map keys lexicographically with no whitespace, matching RFC 7638 §3.1.
	canonical, err := json.Marshal(jwk)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return b64url(sum[:]), nil
}

// JWKThumbprint computes the RFC 7638 SHA-256 thumbprint of an ECDSA key.
func JWKThumbprint(key *ecdsa.PrivateKey) string {
	tp, _ := PublicKeyThumbprint(&key.PublicKey)
	return tp
}

// KeyAuthorization returns "<token>.<thumbprint>" for an ECDSA ACME challenge.
func KeyAuthorization(token string, key *ecdsa.PrivateKey) string {
	return token + "." + JWKThumbprint(key)
}

// SignerKeyAuthorization returns "<token>.<thumbprint>" for any supported ACME account signer (ECDSA or RSA).
func SignerKeyAuthorization(token string, signer crypto.Signer) (string, error) {
	tp, err := PublicKeyThumbprint(signer.Public())
	if err != nil {
		return "", err
	}
	return token + "." + tp, nil
}

// BuildSignerJWS constructs a flattened JWS JSON object (ES256, ES384, or RS256) for RFC 8555 ACME requests.
func BuildSignerJWS(signer crypto.Signer, reqURL, nonce string, payload []byte, kid string) (map[string]string, error) {
	var alg string
	switch k := signer.(type) {
	case *ecdsa.PrivateKey:
		switch k.Curve {
		case elliptic.P256():
			alg = "ES256"
		case elliptic.P384():
			alg = "ES384"
		default:
			return nil, certErrorf("unsupported ECDSA curve for ACME JWS signing")
		}
	case *rsa.PrivateKey:
		if k.N == nil || k.N.BitLen() < 2048 {
			return nil, certErrorf("RSA account key must be at least 2048 bits")
		}
		alg = "RS256"
	default:
		return nil, certErrorf("unsupported ACME account key type %T", signer)
	}

	header := map[string]any{
		"alg":   alg,
		"nonce": nonce,
		"url":   reqURL,
	}
	if kid == "" {
		jwk, err := PublicKeyJWK(signer.Public())
		if err != nil {
			return nil, err
		}
		header["jwk"] = jwk
	} else {
		header["kid"] = kid
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	protected := b64url(headerJSON)
	encodedPayload := ""
	if payload != nil {
		encodedPayload = b64url(payload)
	}
	signingInput := []byte(protected + "." + encodedPayload)

	var sigBytes []byte
	switch k := signer.(type) {
	case *ecdsa.PrivateKey:
		if alg == "ES384" {
			digest := sha512.Sum384(signingInput)
			r, s, signErr := ecdsa.Sign(rand.Reader, k, digest[:])
			if signErr != nil {
				return nil, signErr
			}
			sigBytes = make([]byte, 96)
			r.FillBytes(sigBytes[:48])
			s.FillBytes(sigBytes[48:])
		} else {
			digest := sha256.Sum256(signingInput)
			r, s, signErr := ecdsa.Sign(rand.Reader, k, digest[:])
			if signErr != nil {
				return nil, signErr
			}
			sigBytes = make([]byte, 64)
			r.FillBytes(sigBytes[:32])
			s.FillBytes(sigBytes[32:])
		}
	case *rsa.PrivateKey:
		digest := sha256.Sum256(signingInput)
		sigBytes, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest[:])
		if err != nil {
			return nil, err
		}
	}

	return map[string]string{
		"protected": protected,
		"payload":   encodedPayload,
		"signature": b64url(sigBytes),
	}, nil
}

// BuildJWS constructs a flattened ES256/ES384 JWS JSON object for RFC 8555 ACME requests.
func BuildJWS(key *ecdsa.PrivateKey, reqURL, nonce string, payload []byte, kid string) (map[string]string, error) {
	return BuildSignerJWS(key, reqURL, nonce, payload, kid)
}

// BuildEABJWS constructs a flattened HS256 External Account Binding JWS per RFC 8555 §7.3.4.
func BuildEABJWS(accountPub crypto.PublicKey, eabKID, eabHMACKey, newAccountURL string) (map[string]string, error) {
	trimmedKID := strings.TrimSpace(eabKID)
	if trimmedKID == "" {
		return nil, certErrorf("EAB key ID (kid) must not be empty")
	}
	if strings.TrimSpace(newAccountURL) == "" {
		return nil, certErrorf("ACME newAccount URL must not be empty for EAB")
	}
	macKey, err := decodeEABHMACKey(eabHMACKey)
	if err != nil {
		return nil, err
	}
	jwk, err := PublicKeyJWK(accountPub)
	if err != nil {
		return nil, err
	}
	payloadJSON, err := json.Marshal(jwk)
	if err != nil {
		return nil, err
	}
	header := map[string]string{
		"alg": "HS256",
		"kid": trimmedKID,
		"url": newAccountURL,
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	protected := b64url(headerJSON)
	encodedPayload := b64url(payloadJSON)

	mac := hmac.New(sha256.New, macKey)
	mac.Write([]byte(protected + "." + encodedPayload))
	sig := mac.Sum(nil)

	return map[string]string{
		"protected": protected,
		"payload":   encodedPayload,
		"signature": b64url(sig),
	}, nil
}

// VerifyJWSSignature verifies a flattened ES256 JWS signature against pub.
func VerifyJWSSignature(pub *ecdsa.PublicKey, doc map[string]string) error {
	signingInput := doc["protected"] + "." + doc["payload"]
	rawSig, err := b64urlDecode(doc["signature"])
	if err != nil || len(rawSig) != 64 {
		return certErrorf("invalid JWS signature length")
	}
	r := new(big.Int).SetBytes(rawSig[:32])
	s := new(big.Int).SetBytes(rawSig[32:])
	digest := sha256.Sum256([]byte(signingInput))
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return certErrorf("JWS signature verification failed")
	}
	return nil
}

// LoadOrCreateAccountKey loads an existing ECDSA (P-256/P-384) or RSA account key from path,
// or generates and persists a new ECDSA P-256 account key by default.
func LoadOrCreateAccountKey(path string) (crypto.Signer, error) {
	signer, _, err := loadOrCreateAccountKeyWithState(path)
	return signer, err
}

func loadOrCreateAccountKeyWithState(path string) (signer crypto.Signer, existed bool, err error) {
	if path != "" {
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			parsed, parseErr := ParsePrivateKeyPEM(raw, os.Getenv("GYMBAG_KEY_PASSWORD"))
			if parseErr != nil {
				return nil, false, certErrorf("ACME account key could not be parsed: %v", parseErr)
			}
			if _, jwkErr := PublicKeyJWK(parsed.Public()); jwkErr != nil {
				return nil, false, jwkErr
			}
			return parsed, true, nil
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return nil, false, certErrorf("failed to read ACME account key %s: %v", path, readErr)
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, false, certErrorf("failed to generate ACME account key: %v", err)
	}
	if path != "" {
		pemBytes, mErr := MarshalPrivateKeyPEM(key)
		if mErr != nil {
			return nil, false, mErr
		}
		if wErr := writePrivateFile(path, pemBytes); wErr != nil {
			return nil, false, certErrorf("failed to persist ACME account key: %v", wErr)
		}
	}
	return key, false, nil
}

// LoadOrCreateCertKey loads a user-supplied CSR private key (ECDSA or RSA) from path if it exists,
// generates and saves a new ECDSA P-256 key to path if path is non-empty and missing, or generates
// an ephemeral ECDSA P-256 key when path is empty.
func LoadOrCreateCertKey(path string) (crypto.Signer, []byte, error) {
	if path != "" {
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			signer, parseErr := ParsePrivateKeyPEM(raw, os.Getenv("GYMBAG_KEY_PASSWORD"))
			if parseErr != nil {
				return nil, nil, certErrorf("CSR private key %s could not be parsed: %v", path, parseErr)
			}
			switch k := signer.(type) {
			case *ecdsa.PrivateKey, *rsa.PrivateKey:
				_ = k
			default:
				return nil, nil, certErrorf("CSR private key must be ECDSA or RSA")
			}
			pemBytes, mErr := MarshalPrivateKeyPEM(signer)
			if mErr != nil {
				return nil, nil, mErr
			}
			return signer, pemBytes, nil
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return nil, nil, certErrorf("failed to read CSR private key %s: %v", path, readErr)
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, certErrorf("failed to generate certificate key: %v", err)
	}
	pemBytes, err := MarshalPrivateKeyPEM(key)
	if err != nil {
		return nil, nil, err
	}
	if path != "" {
		if err := writePrivateFile(path, pemBytes); err != nil {
			return nil, nil, certErrorf("failed to persist CSR private key: %v", err)
		}
	}
	return key, pemBytes, nil
}

// RejectIneligibleACME validates that domains and email are eligible for ACME HTTP-01 issuance.
func RejectIneligibleACME(domains []string, email string) error {
	return RejectIneligibleACMEWithOptions(domains, email, false)
}

// RejectIneligibleACMEWithOptions validates domains/IPs and email for ACME HTTP-01 issuance.
func RejectIneligibleACMEWithOptions(identifiers []string, email string, allowIPs bool) error {
	if email != "" && (!strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@")) {
		return certErrorf("GYMBAG_ACME_EMAIL must be a valid email address")
	}
	if len(identifiers) == 0 {
		return certErrorf("ACME requires at least one public domain")
	}
	var reasons []string
	for _, d := range identifiers {
		if strings.HasPrefix(d, "*.") {
			reasons = append(reasons, fmt.Sprintf("%s needs DNS-01; Gymbag completes HTTP-01 only", d))
		} else if !isPublicACMEIdentifier(d, allowIPs) {
			reasons = append(reasons, fmt.Sprintf("%s is not a public hostname", d))
		}
	}
	if len(reasons) > 0 {
		return certErrorf("%s", strings.Join(reasons, "; "))
	}
	return nil
}

// ACMEOptions configures multi-CA ACME issuance, RFC 8555 External Account Binding, ZeroSSL auto-EAB,
// and user-supplied account or CSR private keys.
type ACMEOptions struct {
	ProviderName   string
	DirectoryURL   string
	AccountKeyPath string // user can supply their own EC or RSA account PEM key, or auto-create ECDSA P-256
	CertKeyPath    string // optional: user can supply their own private key PEM to sign the CSR!
	EABKID         string // External Account Binding Key ID
	EABHMACKey     string // External Account Binding base64url (or standard base64) HMAC key
	AutoEABURL     string // e.g. ZeroSSL email-to-EAB endpoint when EABKID == "" && email != ""
	HTTPClient     *http.Client
}

// ACMEClient requests TLS certificates via the RFC 8555 ACME HTTP-01 protocol.
type ACMEClient struct {
	providerName   string
	directoryURL   string
	accountKeyPath string
	certKeyPath    string
	eabKID         string
	eabHMACKey     string
	autoEABURL     string
	httpClient     *http.Client

	mu        sync.Mutex
	directory map[string]string
	profiles  map[string]string
	nonce     string
	kid       string
	key       crypto.Signer
}

func defaultACMEHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient != nil {
		return httpClient
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// NewACMEClient constructs an ACMEClient using httpClient (or a default 30s client if nil).
func NewACMEClient(directoryURL, accountKeyPath string, httpClient *http.Client) *ACMEClient {
	return NewACMEClientWithOptions(ACMEOptions{
		DirectoryURL:   directoryURL,
		AccountKeyPath: accountKeyPath,
		HTTPClient:     httpClient,
	})
}

// NewACMEClientWithOptions constructs an ACMEClient from ACMEOptions, resolving catalog defaults when applicable.
func NewACMEClientWithOptions(opts ACMEOptions) *ACMEClient {
	provName := strings.TrimSpace(opts.ProviderName)
	dirURL := strings.TrimSpace(opts.DirectoryURL)
	autoEABURL := strings.TrimSpace(opts.AutoEABURL)

	if provName != "" {
		if info, err := ResolveSSLProvider(provName); err == nil {
			if info.Name != "custom" {
				provName = info.Name
			}
			if dirURL == "" {
				dirURL = info.DirectoryURL
				if autoEABURL == "" && info.SupportsAutoEAB {
					autoEABURL = info.AutoEABURL
				}
			} else if strings.EqualFold(strings.TrimRight(dirURL, "/"), strings.TrimRight(info.DirectoryURL, "/")) {
				if autoEABURL == "" && info.SupportsAutoEAB {
					autoEABURL = info.AutoEABURL
				}
			}
		}
	} else if dirURL != "" {
		if info, ok := inferProviderFromDirectoryURL(dirURL); ok {
			provName = info.Name
			if autoEABURL == "" && info.SupportsAutoEAB {
				autoEABURL = info.AutoEABURL
			}
		}
	}

	if dirURL == "" {
		dirURL = LetsEncryptDirectoryURL
		if provName == "" {
			provName = "letsencrypt"
		}
	}

	return &ACMEClient{
		providerName:   provName,
		directoryURL:   dirURL,
		accountKeyPath: opts.AccountKeyPath,
		certKeyPath:    opts.CertKeyPath,
		eabKID:         strings.TrimSpace(opts.EABKID),
		eabHMACKey:     strings.TrimSpace(opts.EABHMACKey),
		autoEABURL:     autoEABURL,
		httpClient:     defaultACMEHTTPClient(opts.HTTPClient),
	}
}

// ProviderName returns the normalized SSL provider name associated with this client.
func (c *ACMEClient) ProviderName() string {
	return c.providerName
}

// DirectoryURL returns the ACME v2 directory endpoint URL used by this client.
func (c *ACMEClient) DirectoryURL() string {
	return c.directoryURL
}

// Issue executes a full ACME HTTP-01 certificate order for domains.
func (c *ACMEClient) Issue(ctx context.Context, domains []string, email string, board *HTTP01Board) (certPEM, keyPEM []byte, err error) {
	return c.IssueWithOptions(ctx, domains, email, board, false)
}

// IssueWithOptions executes an ACME HTTP-01 certificate order for domains and (when allowIPs) RFC 8738 public IPs.
func (c *ACMEClient) IssueWithOptions(ctx context.Context, identifiers []string, email string, board *HTTP01Board, allowIPs bool) (certPEM, keyPEM []byte, err error) {
	if err := RejectIneligibleACMEWithOptions(identifiers, email, allowIPs); err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key, existed, err := loadOrCreateAccountKeyWithState(c.accountKeyPath)
	if err != nil {
		return nil, nil, err
	}
	c.key = key

	if _, err := c.loadDirectoryLocked(ctx); err != nil {
		return nil, nil, err
	}
	if err := c.registerLocked(ctx, email, existed); err != nil {
		return nil, nil, err
	}
	order, orderURL, err := c.newOrderLocked(ctx, identifiers)
	if err != nil {
		return nil, nil, err
	}
	authzURLs, _ := toStringSlice(order["authorizations"])
	for _, authzURL := range authzURLs {
		if err := c.satisfyHTTP01Locked(ctx, authzURL, board); err != nil {
			return nil, nil, err
		}
	}

	certSigner, certKeyPEM, err := LoadOrCreateCertKey(c.certKeyPath)
	if err != nil {
		return nil, nil, err
	}
	csrDER, err := buildCSR(certSigner, identifiers)
	if err != nil {
		return nil, nil, err
	}
	finalizeURL, _ := order["finalize"].(string)
	csrPayload, _ := json.Marshal(map[string]string{"csr": b64url(csrDER)})
	finalized, err := c.signedJSONLocked(ctx, finalizeURL, csrPayload)
	if err != nil {
		return nil, nil, err
	}
	if status, _ := finalized["status"].(string); status != "valid" {
		finalized, err = c.pollLocked(ctx, orderURL)
		if err != nil {
			return nil, nil, err
		}
	}
	if status, _ := finalized["status"].(string); status != "valid" {
		return nil, nil, certErrorf("ACME order failed")
	}
	certURL, _ := finalized["certificate"].(string)
	if certURL == "" {
		return nil, nil, certErrorf("ACME order did not include a certificate")
	}
	statusCode, bodyBytes, _, err := c.signedLocked(ctx, certURL, nil)
	if err != nil {
		return nil, nil, err
	}
	if statusCode >= 400 || !bytes.Contains(bodyBytes, []byte("BEGIN CERTIFICATE")) {
		return nil, nil, certErrorf("%s", acmeProblem(statusCode, bodyBytes))
	}

	return bodyBytes, certKeyPEM, nil
}

func (c *ACMEClient) loadDirectoryLocked(ctx context.Context) (map[string]string, error) {
	if c.directory != nil {
		return c.directory, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.directoryURL, nil)
	if err != nil {
		return nil, certErrorf("ACME request failed: %v", err)
	}
	req.Header.Set("User-Agent", "gymbag/0.1.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, certErrorf("ACME request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, certErrorf("%s", acmeProblem(resp.StatusCode, body))
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, certErrorf("ACME directory was not an object")
	}
	dir := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			dir[k] = s
		}
	}
	if meta, ok := raw["meta"].(map[string]any); ok {
		if profs, ok := meta["profiles"].(map[string]any); ok {
			c.profiles = make(map[string]string, len(profs))
			for pk, pv := range profs {
				if ps, ok := pv.(string); ok {
					c.profiles[pk] = ps
				}
			}
		}
	}
	c.directory = dir
	return dir, nil
}

func (c *ACMEClient) provisionAutoEABLocked(ctx context.Context, email string) error {
	if c.eabKID != "" || c.autoEABURL == "" || email == "" {
		return nil
	}
	form := url.Values{"email": {email}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.autoEABURL, strings.NewReader(form))
	if err != nil {
		return certErrorf("auto-EAB request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "gymbag/0.1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return certErrorf("auto-EAB request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return certErrorf("auto-EAB request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Success    *bool  `json:"success"`
		EABKID     string `json:"eab_kid"`
		EABHMACKey string `json:"eab_hmac_key"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return certErrorf("auto-EAB response was not valid JSON")
	}
	if parsed.Success != nil && !*parsed.Success {
		return certErrorf("auto-EAB provisioning rejected: %s", strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(parsed.EABKID) == "" || strings.TrimSpace(parsed.EABHMACKey) == "" {
		return certErrorf("auto-EAB response did not include eab_kid and eab_hmac_key")
	}
	c.eabKID = strings.TrimSpace(parsed.EABKID)
	c.eabHMACKey = strings.TrimSpace(parsed.EABHMACKey)
	return nil
}

func (c *ACMEClient) registerLocked(ctx context.Context, email string, accountKeyExisted bool) error {
	dir, err := c.loadDirectoryLocked(ctx)
	if err != nil {
		return err
	}
	if err := c.provisionAutoEABLocked(ctx, email); err != nil {
		return err
	}
	if (c.eabKID == "") != (c.eabHMACKey == "") {
		return certErrorf("ACME External Account Binding requires both EAB KID and EAB HMAC key")
	}

	newAccountURL := dir["newAccount"]
	acctPayload := map[string]any{"termsOfServiceAgreed": true}
	if email != "" {
		acctPayload["contact"] = []string{"mailto:" + email}
	}
	hasEAB := c.eabKID != "" && c.eabHMACKey != ""
	if hasEAB {
		eabObj, eabErr := BuildEABJWS(c.key.Public(), c.eabKID, c.eabHMACKey, newAccountURL)
		if eabErr != nil {
			return eabErr
		}
		acctPayload["externalAccountBinding"] = eabObj
	}

	payloadBytes, _ := json.Marshal(acctPayload)
	c.kid = ""
	statusCode, body, hdrs, err := c.signedLocked(ctx, newAccountURL, payloadBytes)
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated && hasEAB && accountKeyExisted {
		// Single-use EAB token may already be bound to this existing account key; try lookup-only per RFC 8555 §7.3.1.
		existingPayload, _ := json.Marshal(map[string]any{"onlyReturnExisting": true})
		if sc2, body2, hdrs2, err2 := c.signedLocked(ctx, newAccountURL, existingPayload); err2 == nil && (sc2 == http.StatusOK || sc2 == http.StatusCreated) {
			statusCode, body, hdrs = sc2, body2, hdrs2
		}
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return certErrorf("%s", acmeProblem(statusCode, body))
	}
	loc := hdrs.Get("Location")
	if loc == "" {
		return certErrorf("ACME account response did not include a location")
	}
	c.kid = resolveURL(newAccountURL, loc)
	return nil
}

func (c *ACMEClient) newOrderLocked(ctx context.Context, identifiers []string) (map[string]any, string, error) {
	dir, err := c.loadDirectoryLocked(ctx)
	if err != nil {
		return nil, "", err
	}
	ids := make([]map[string]string, len(identifiers))
	hasIP := false
	for i, d := range identifiers {
		norm := host.Normalize(d)
		if addr, parseErr := netip.ParseAddr(norm); parseErr == nil {
			ids[i] = map[string]string{"type": "ip", "value": addr.Unmap().String()}
			hasIP = true
		} else {
			ids[i] = map[string]string{"type": "dns", "value": norm}
		}
	}
	orderReq := map[string]any{"identifiers": ids}
	if hasIP {
		if _, hasShortlived := c.profiles["shortlived"]; hasShortlived {
			orderReq["profile"] = "shortlived"
		}
	}
	payloadBytes, _ := json.Marshal(orderReq)
	statusCode, body, hdrs, err := c.signedLocked(ctx, dir["newOrder"], payloadBytes)
	if err != nil {
		return nil, "", err
	}
	if statusCode >= 400 {
		return nil, "", certErrorf("%s", acmeProblem(statusCode, body))
	}
	var order map[string]any
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, "", certErrorf("ACME order response was not an object")
	}
	loc := hdrs.Get("Location")
	if loc == "" {
		return nil, "", certErrorf("ACME order response did not include a location")
	}
	return order, resolveURL(dir["newOrder"], loc), nil
}

func (c *ACMEClient) satisfyHTTP01Locked(ctx context.Context, authzURL string, board *HTTP01Board) error {
	authz, err := c.signedJSONLocked(ctx, authzURL, nil)
	if err != nil {
		return err
	}
	if status, _ := authz["status"].(string); status == "valid" {
		return nil
	}
	rawChalls, _ := authz["challenges"].([]any)
	var token, challURL string
	for _, item := range rawChalls {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == "http-01" {
			token, _ = m["token"].(string)
			challURL, _ = m["url"].(string)
			break
		}
	}
	if token == "" || challURL == "" {
		return certErrorf("ACME authorization did not offer an HTTP-01 challenge")
	}
	keyAuth, err := SignerKeyAuthorization(token, c.key)
	if err != nil {
		return err
	}
	if board != nil {
		board.Publish(token, keyAuth)
	}
	if _, err := c.signedJSONLocked(ctx, challURL, []byte("{}")); err != nil {
		return err
	}
	completed, err := c.pollLocked(ctx, authzURL)
	if err != nil {
		return err
	}
	if status, _ := completed["status"].(string); status != "valid" {
		return certErrorf("ACME HTTP-01 challenge failed")
	}
	return nil
}

func (c *ACMEClient) pollLocked(ctx context.Context, targetURL string) (map[string]any, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		payload, err := c.signedJSONLocked(ctx, targetURL, nil)
		if err != nil {
			return nil, err
		}
		status, _ := payload["status"].(string)
		if status == "valid" || status == "invalid" {
			return payload, nil
		}
		if time.Now().After(deadline) {
			return nil, certErrorf("timed out waiting for the ACME order")
		}
		select {
		case <-ctx.Done():
			return nil, certErrorf("ACME polling cancelled: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func (c *ACMEClient) signedJSONLocked(ctx context.Context, targetURL string, payload []byte) (map[string]any, error) {
	statusCode, body, _, err := c.signedLocked(ctx, targetURL, payload)
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, certErrorf("%s", acmeProblem(statusCode, body))
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, certErrorf("ACME response was not an object")
	}
	return out, nil
}

func (c *ACMEClient) signedLocked(ctx context.Context, targetURL string, payload []byte) (int, []byte, http.Header, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if c.nonce == "" {
			if err := c.refreshNonceLocked(ctx); err != nil {
				return 0, nil, nil, err
			}
		}
		doc, err := BuildSignerJWS(c.key, targetURL, c.nonce, payload, c.kid)
		if err != nil {
			return 0, nil, nil, err
		}
		docBytes, _ := json.Marshal(doc)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(docBytes))
		if err != nil {
			return 0, nil, nil, certErrorf("ACME request failed: %v", err)
		}
		req.Header.Set("Content-Type", "application/jose+json")
		req.Header.Set("User-Agent", "gymbag/0.1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, nil, certErrorf("ACME request failed: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		c.nonce = resp.Header.Get("Replay-Nonce")
		if resp.StatusCode == http.StatusBadRequest && bytes.Contains(body, []byte("badNonce")) {
			continue
		}
		return resp.StatusCode, body, resp.Header, nil
	}
	return 0, nil, nil, certErrorf("ACME request failed due to repeated badNonce")
}

func (c *ACMEClient) refreshNonceLocked(ctx context.Context) error {
	dir, err := c.loadDirectoryLocked(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dir["newNonce"], nil)
	if err != nil {
		return certErrorf("ACME request failed: %v", err)
	}
	req.Header.Set("User-Agent", "gymbag/0.1.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return certErrorf("ACME request failed: %v", err)
	}
	_ = resp.Body.Close()
	nonce := resp.Header.Get("Replay-Nonce")
	if nonce == "" {
		return certErrorf("ACME directory did not return a nonce")
	}
	c.nonce = nonce
	return nil
}

func buildCSR(key crypto.Signer, identifiers []string) ([]byte, error) {
	var dnsNames []string
	var ipAddrs []net.IP
	for _, raw := range identifiers {
		norm := host.Normalize(raw)
		if addr, err := netip.ParseAddr(norm); err == nil {
			unmapped := addr.Unmap()
			ip := net.IP(unmapped.AsSlice())
			if ip4 := ip.To4(); ip4 != nil {
				ip = ip4
			}
			ipAddrs = append(ipAddrs, ip)
		} else if norm != "" {
			dnsNames = append(dnsNames, norm)
		}
	}
	tmpl := &x509.CertificateRequest{
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
	}
	if len(dnsNames) > 0 {
		cn := dnsNames[0]
		if len(cn) > 64 {
			cn = cn[:64]
		}
		tmpl.Subject = pkix.Name{CommonName: cn}
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, certErrorf("failed to create CSR: %v", err)
	}
	return der, nil
}

func acmeProblem(statusCode int, body []byte) string {
	detail := string(body)
	if len(detail) > 300 {
		detail = detail[:300]
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		if d, ok := m["detail"].(string); ok && d != "" {
			detail = d
		} else if t, ok := m["type"].(string); ok && t != "" {
			detail = t
		}
	}
	return fmt.Sprintf("ACME request failed (%d): %s", statusCode, detail)
}

func resolveURL(baseStr, refStr string) string {
	base, err1 := url.Parse(baseStr)
	ref, err2 := url.Parse(refStr)
	if err1 != nil || err2 != nil {
		return refStr
	}
	return base.ResolveReference(ref).String()
}

func toStringSlice(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out, true
}
