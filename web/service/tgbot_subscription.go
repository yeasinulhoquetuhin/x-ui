package service

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/web/global"
)

// PublicSubscriptionHost resolves a public endpoint, never the OS hostname.
// In particular, an HTTPS fallback must use a name on the certificate.
func (t *Tgbot) PublicSubscriptionHost() (string, error) {
	if uri, _ := t.settingService.GetSubURI(); uri != "" {
		u, err := url.Parse(uri)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return "", errors.New("invalid subscription URL; configure an absolute HTTP(S) URL")
		}
		return u.Hostname(), nil
	}
	if domain, _ := t.settingService.GetSubDomain(); domain != "" {
		return domain, nil
	}
	webDomain, _ := t.settingService.GetWebDomain()
	certFile, _ := t.settingService.GetSubCertFile()
	keyFile, _ := t.settingService.GetSubKeyFile()
	if certFile != "" && keyFile != "" {
		data, err := os.ReadFile(certFile)
		if err != nil {
			return "", err
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return "", errors.New("invalid subscription certificate")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", err
		}
		if webDomain != "" && cert.VerifyHostname(webDomain) == nil {
			return webDomain, nil
		}
		for _, name := range cert.DNSNames {
			if !strings.Contains(name, "*") && name != "localhost" {
				return name, nil
			}
		}
		for _, ip := range cert.IPAddresses {
			if ip.IsGlobalUnicast() && !ip.IsPrivate() {
				return ip.String(), nil
			}
		}
		return "", errors.New("set a subscription domain matching the TLS certificate")
	}
	if webDomain != "" {
		return webDomain, nil
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	var ipv6 string
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			continue
		}
		if ip.To4() != nil {
			return ip.String(), nil
		}
		ipv6 = ip.String()
	}
	if ipv6 != "" {
		return ipv6, nil
	}
	return "", errors.New("set a public subscription domain or URL")
}

// ClientIndividualLinks uses the same local API/service as subscriptions.
// It does not fetch our own public HTTPS URL, so DNS, NAT loopback, certificate
// names and public listener timeouts cannot break bot configuration generation.
func (t *Tgbot) ClientIndividualLinks(email string) ([]string, error) {
	_, client, err := t.inboundService.GetClientByEmail(email)
	if err != nil || client == nil || client.SubID == "" {
		return nil, errors.New("client subscription not found")
	}
	host, err := t.PublicSubscriptionHost()
	if err != nil {
		return nil, err
	}
	provider, ok := global.GetSubServer().(interface {
		GetSubscriptionLinks(string, string) ([]string, error)
	})
	if !ok {
		return nil, errors.New("local subscription service is unavailable")
	}
	return provider.GetSubscriptionLinks(client.SubID, host)
}
