package tlsmanager

import (
	"net/url"
	"slices"
	"strings"
)

const (
	LetsEncryptDirectoryURL                = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStagingDirectoryURL         = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ZeroSSLDirectoryURL                    = "https://acme.zerossl.com/v2/DV90"
	ZeroSSLAutoEABURL                      = "https://api.zerossl.com/acme/eab-credentials-email"
	GoogleTrustServicesDirectoryURL        = "https://dv.acme-v02.api.pki.goog/directory"
	GoogleTrustServicesStagingDirectoryURL = "https://dv.acme-staging-v02.api.pki.goog/directory"
	SSLComECCDirectoryURL                  = "https://acme.ssl.com/sslcom-dv-ecc"
	SSLComRSADirectoryURL                  = "https://acme.ssl.com/sslcom-dv-rsa"
	LiteSSLDirectoryURL                    = "https://acme.litessl.com/acme/v2/directory"
	BuypassDirectoryURL                    = "https://api.buypass.com/acme/directory"
)

// CAProviderInfo describes an ACME v2 Certificate Authority endpoint and its External Account Binding capabilities.
type CAProviderInfo struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	DirectoryURL    string `json:"directory_url"`
	RequiresEAB     bool   `json:"requires_eab"`
	SupportsAutoEAB bool   `json:"supports_auto_eab"`
	AutoEABURL      string `json:"auto_eab_url,omitempty"`
}

var sslProvidersCatalog = []CAProviderInfo{
	{
		Name:         "letsencrypt",
		DisplayName:  "Let's Encrypt",
		DirectoryURL: LetsEncryptDirectoryURL,
	},
	{
		Name:         "letsencrypt_staging",
		DisplayName:  "Let's Encrypt (Staging)",
		DirectoryURL: LetsEncryptStagingDirectoryURL,
	},
	{
		Name:            "zerossl",
		DisplayName:     "ZeroSSL",
		DirectoryURL:    ZeroSSLDirectoryURL,
		RequiresEAB:     true,
		SupportsAutoEAB: true,
		AutoEABURL:      ZeroSSLAutoEABURL,
	},
	{
		Name:         "google",
		DisplayName:  "Google Trust Services",
		DirectoryURL: GoogleTrustServicesDirectoryURL,
		RequiresEAB:  true,
	},
	{
		Name:         "ssl.com",
		DisplayName:  "SSL.com (ECC)",
		DirectoryURL: SSLComECCDirectoryURL,
		RequiresEAB:  true,
	},
	{
		Name:         "ssl.com-rsa",
		DisplayName:  "SSL.com (RSA)",
		DirectoryURL: SSLComRSADirectoryURL,
		RequiresEAB:  true,
	},
	{
		Name:         "litessl",
		DisplayName:  "LiteSSL (TrustAsia)",
		DirectoryURL: LiteSSLDirectoryURL,
	},
	{
		Name:         "buypass",
		DisplayName:  "Buypass",
		DirectoryURL: BuypassDirectoryURL,
	},
}

// ListSSLProviders returns a copy of all built-in ACME Certificate Authority providers.
func ListSSLProviders() []CAProviderInfo {
	return slices.Clone(sslProvidersCatalog)
}

// ResolveSSLProvider resolves a case-insensitive CA provider name, alias, or custom http(s):// ACME directory URL.
func ResolveSSLProvider(nameOrURL string) (CAProviderInfo, error) {
	trimmed := strings.TrimSpace(nameOrURL)
	lower := strings.ToLower(trimmed)

	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		if strings.ContainsAny(trimmed, " \t\r\n") {
			return CAProviderInfo{}, certErrorf("invalid custom ACME directory URL %q", nameOrURL)
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" {
			return CAProviderInfo{}, certErrorf("invalid custom ACME directory URL %q", nameOrURL)
		}
		return CAProviderInfo{
			Name:         "custom",
			DisplayName:  "Custom ACME CA",
			DirectoryURL: parsed.String(),
		}, nil
	}

	switch lower {
	case "", "default", "letsencrypt", "le", "lets_encrypt", "lets-encrypt":
		return providerByName("letsencrypt"), nil
	case "letsencrypt_staging", "letsencrypt-staging", "le_staging", "le-staging":
		return providerByName("letsencrypt_staging"), nil
	case "zerossl", "zero_ssl", "zero-ssl":
		return providerByName("zerossl"), nil
	case "google", "gts", "google_trust_services", "google-trust-services":
		return providerByName("google"), nil
	case "google_staging", "google-staging", "gts_staging", "gts-staging":
		return CAProviderInfo{
			Name:         "google_staging",
			DisplayName:  "Google Trust Services (Staging)",
			DirectoryURL: GoogleTrustServicesStagingDirectoryURL,
			RequiresEAB:  true,
		}, nil
	case "ssl.com", "sslcom", "ssl_com", "ssl.com-ecc", "sslcom-ecc":
		return providerByName("ssl.com"), nil
	case "ssl.com-rsa", "sslcom-rsa", "ssl_com_rsa", "ssl.com_rsa":
		return providerByName("ssl.com-rsa"), nil
	case "litessl", "lite_ssl", "lite-ssl", "trustasia", "trust_asia", "trust-asia":
		return providerByName("litessl"), nil
	case "buypass", "buypass_go", "buypass-go":
		return providerByName("buypass"), nil
	default:
		return CAProviderInfo{}, certErrorf("unsupported SSL provider %q", nameOrURL)
	}
}

func providerByName(name string) CAProviderInfo {
	for _, p := range sslProvidersCatalog {
		if p.Name == name {
			return p
		}
	}
	return sslProvidersCatalog[0]
}

func inferProviderFromDirectoryURL(dirURL string) (CAProviderInfo, bool) {
	norm := strings.TrimRight(strings.TrimSpace(dirURL), "/")
	if norm == "" {
		return CAProviderInfo{}, false
	}
	for _, p := range sslProvidersCatalog {
		if strings.EqualFold(strings.TrimRight(p.DirectoryURL, "/"), norm) {
			return p, true
		}
	}
	if strings.EqualFold(strings.TrimRight(GoogleTrustServicesStagingDirectoryURL, "/"), norm) {
		return CAProviderInfo{
			Name:         "google_staging",
			DisplayName:  "Google Trust Services (Staging)",
			DirectoryURL: GoogleTrustServicesStagingDirectoryURL,
			RequiresEAB:  true,
		}, true
	}
	return CAProviderInfo{}, false
}
