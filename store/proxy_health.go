package store

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// TCPHealthCheckProxyURL 分配前轻量检测：对代理 host:port 做 TCP 连接（不鉴权 SOCKS 握手）
func TCPHealthCheckProxyURL(proxyURL string, timeout time.Duration) error {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("proxy url missing host")
	}
	port := u.Port()
	if port == "" {
		port = "1080"
	}
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
