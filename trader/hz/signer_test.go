package hz

import "testing"

func TestSignMatchesHZFrozenVector(t *testing.T) {
	body := []byte(`{"clientOrderId":"ai-0001","instrument":"XAUUSD","side":"LONG","orderType":"MARKET","sizeMode":"LOTS","size":"0.010","leverage":500,"marginMode":"CROSS"}`)
	payload := canonicalPayload(
		"1784956800456",
		"node-go-0003",
		"POST",
		"/api/v1/orders",
		"",
		body,
	)
	wantPayload := "1784956800456\nnode-go-0003\nPOST\n/api/v1/orders\n\nd9059115ec4abc880141f016ecf92710f69494e5cc0a000e3f4b26a812f0b215"
	if payload != wantPayload {
		t.Fatalf("canonical payload mismatch:\n%s", payload)
	}
	if got := sign("hz_test_secret_2026", payload); got != "1e269e907ae95d99b3cf0b3e3851ef67a6dd90c96fc5c0b28a1891c2018ee87a" {
		t.Fatalf("signature mismatch: %s", got)
	}
}

func TestCanonicalQuerySortsAndEscapesRFC3986(t *testing.T) {
	got := canonicalQuery("z=%21&a=%E9%BB%84%E9%87%91&a=1&space=a+b")
	want := "a=1&a=%E9%BB%84%E9%87%91&space=a%20b&z=%21"
	if got != want {
		t.Fatalf("canonical query mismatch: %s", got)
	}
}
