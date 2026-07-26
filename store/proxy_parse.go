package store

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// ParseProxyImportLine 解析管理员导入的一行代理。
// 支持：socks5://user:pass@host:port 或 host|port|user|pass|到期时间(可选)
// 到期时间格式：2006-01-02 15:04:05 或 2006-01-02
func ParseProxyImportLine(line string) (socksURL string, displayHost string, expiresAt *time.Time, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", nil, fmt.Errorf("空行")
	}
	if strings.HasPrefix(strings.ToLower(line), "socks5://") || strings.HasPrefix(strings.ToLower(line), "socks5h://") ||
		strings.HasPrefix(strings.ToLower(line), "http://") || strings.HasPrefix(strings.ToLower(line), "https://") {
		u, perr := url.Parse(line)
		if perr != nil {
			return "", "", nil, perr
		}
		host := u.Hostname()
		if host != "" {
			displayHost = host
		}
		return line, displayHost, nil, nil
	}
	parts := strings.Split(line, "|")
	if len(parts) < 4 {
		return "", "", nil, fmt.Errorf("格式应为 host|port|user|pass 或完整 socks5:// URL")
	}
	host := strings.TrimSpace(parts[0])
	port := strings.TrimSpace(parts[1])
	user := strings.TrimSpace(parts[2])
	pass := strings.TrimSpace(parts[3])
	if host == "" || port == "" {
		return "", "", nil, fmt.Errorf("host/port 为空")
	}
	displayHost = host
	u := &url.URL{
		Scheme: "socks5",
		User:     url.UserPassword(user, pass),
		Host:     net.JoinHostPort(host, port),
	}
	socksURL = u.String()

	if len(parts) >= 5 {
		ts := strings.TrimSpace(strings.Join(parts[4:], "|"))
		if ts != "" {
			layouts := []string{
				"2006-01-02 15:04:05",
				"2006-01-02 15:04",
				"2006-01-02",
			}
			var t time.Time
			var ok bool
			for _, lay := range layouts {
				if tt, e := time.ParseInLocation(lay, ts, time.Local); e == nil {
					t = tt
					ok = true
					break
				}
			}
			if ok {
				expiresAt = &t
			}
		}
	}
	return socksURL, displayHost, expiresAt, nil
}
