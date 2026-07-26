package proxyhttp

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

func Client(proxyRaw string, timeout time.Duration) (*http.Client, error) {
	tr, err := Transport(proxyRaw)
	return &http.Client{Transport: tr, Timeout: timeout}, err
}

func Transport(proxyRaw string) (*http.Transport, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	proxyRaw = strings.TrimSpace(proxyRaw)
	if proxyRaw == "" {
		return tr, nil
	}
	u, err := url.Parse(proxyRaw)
	if err != nil {
		return tr, err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		tr.Proxy = http.ProxyURL(u)
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			return tr, err
		}
		cd, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return tr, fmt.Errorf("socks5 dialer does not support context")
		}
		tr.DialContext = cd.DialContext
	default:
		return tr, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
	}
	return tr, nil
}
