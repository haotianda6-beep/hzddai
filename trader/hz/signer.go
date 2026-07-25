package hz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

func canonicalPayload(timestamp, nonce, method, path, query string, body []byte) string {
	sum := sha256.Sum256(body)
	return strings.Join([]string{
		timestamp,
		nonce,
		strings.ToUpper(method),
		path,
		canonicalQuery(query),
		hex.EncodeToString(sum[:]),
	}, "\n")
}

func canonicalQuery(raw string) string {
	values, _ := url.ParseQuery(raw)
	type pair struct{ key, value string }
	pairs := make([]pair, 0, len(values))
	for key, items := range values {
		for _, value := range items {
			pairs = append(pairs, pair{key, value})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})
	parts := make([]string, len(pairs))
	for i, item := range pairs {
		parts[i] = rfc3986(item.key) + "=" + rfc3986(item.value)
	}
	return strings.Join(parts, "&")
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func rfc3986(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
