package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type binanceBrokerRebateRow struct {
	Market       string                 `json:"market"`
	Asset        string                 `json:"asset"`
	Amount       float64                `json:"amount"`
	CustomerID   string                 `json:"customer_id,omitempty"`
	SubAccountID string                 `json:"sub_account_id,omitempty"`
	Symbol       string                 `json:"symbol,omitempty"`
	IncomeType   string                 `json:"income_type,omitempty"`
	Time         int64                  `json:"time,omitempty"`
	Raw          map[string]interface{} `json:"raw,omitempty"`
}

func (s *Server) handleAdminBinanceBrokerRebates(c *gin.Context) {
	apiKey := strings.TrimSpace(os.Getenv("BINANCE_BROKER_API_KEY"))
	secret := strings.TrimSpace(os.Getenv("BINANCE_BROKER_API_SECRET"))
	if apiKey == "" || secret == "" {
		c.JSON(http.StatusOK, gin.H{
			"configured": false,
			"message":    "未配置 BINANCE_BROKER_API_KEY / BINANCE_BROKER_API_SECRET",
			"items":      []binanceBrokerRebateRow{},
			"totals":     gin.H{},
		})
		return
	}
	limit := 100
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	var all []binanceBrokerRebateRow
	var sourceErrors []string
	for _, spec := range []struct {
		market string
		path   string
		params map[string]string
	}{
		{"spot", "/sapi/v1/broker/rebate/recentRecord", nil},
		{"futures", "/sapi/v1/broker/rebate/futures/recentRecord", map[string]string{
			"futuresType": "1",
			"startTime":   strconv.FormatInt(time.Now().AddDate(0, 0, -30).UnixMilli(), 10),
			"endTime":     strconv.FormatInt(time.Now().UnixMilli(), 10),
		}},
	} {
		rows, err := fetchBinanceBrokerRebateRows(apiKey, secret, spec.market, spec.path, limit, spec.params)
		if err != nil {
			sourceErrors = append(sourceErrors, spec.market+": "+err.Error())
			continue
		}
		all = append(all, rows...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Time > all[j].Time })
	totals := map[string]float64{}
	for _, r := range all {
		if r.Asset == "" {
			r.Asset = "UNKNOWN"
		}
		totals[r.Asset] += r.Amount
	}
	c.JSON(http.StatusOK, gin.H{
		"configured":   true,
		"items":        all,
		"totals":       totals,
		"errors":       sourceErrors,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func fetchBinanceBrokerRebateRows(apiKey, secret, market, path string, limit int, extra map[string]string) ([]binanceBrokerRebateRow, error) {
	q := url.Values{}
	q.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	q.Set("recvWindow", "5000")
	q.Set("limit", strconv.Itoa(limit))
	for k, v := range extra {
		q.Set(k, v)
	}
	payload := q.Encode()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	u := "https://api.binance.com" + path + "?" + payload + "&signature=" + sig
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", apiKey)
	req.Header.Set("User-Agent", "COMKUN-AI-broker-rebate/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(b, &arr); err != nil {
		var wrapped struct {
			Data []map[string]interface{} `json:"data"`
			Rows []map[string]interface{} `json:"rows"`
		}
		if err2 := json.Unmarshal(b, &wrapped); err2 != nil {
			return nil, err
		}
		if len(wrapped.Data) > 0 {
			arr = wrapped.Data
		} else {
			arr = wrapped.Rows
		}
	}
	out := make([]binanceBrokerRebateRow, 0, len(arr))
	for _, raw := range arr {
		out = append(out, normalizeBrokerRebateRow(market, raw))
	}
	return out, nil
}

func normalizeBrokerRebateRow(market string, raw map[string]interface{}) binanceBrokerRebateRow {
	getS := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok && v != nil {
				return strings.TrimSpace(fmt.Sprintf("%v", v))
			}
		}
		return ""
	}
	getF := func(keys ...string) float64 {
		for _, k := range keys {
			if v, ok := raw[k]; ok && v != nil {
				switch t := v.(type) {
				case float64:
					return t
				case string:
					f, _ := strconv.ParseFloat(t, 64)
					return f
				default:
					f, _ := strconv.ParseFloat(fmt.Sprintf("%v", t), 64)
					return f
				}
			}
		}
		return 0
	}
	getI := func(keys ...string) int64 {
		for _, k := range keys {
			if v, ok := raw[k]; ok && v != nil {
				switch t := v.(type) {
				case float64:
					return int64(t)
				case string:
					n, _ := strconv.ParseInt(t, 10, 64)
					return n
				default:
					n, _ := strconv.ParseInt(fmt.Sprintf("%v", t), 10, 64)
					return n
				}
			}
		}
		return 0
	}
	return binanceBrokerRebateRow{
		Market:       market,
		Asset:        getS("asset", "rebateAsset", "incomeAsset", "commissionAsset"),
		Amount:       getF("amount", "rebateAmount", "income", "commission", "commissionAmount"),
		CustomerID:   getS("customerId", "customerID"),
		SubAccountID: getS("subAccountId", "subaccountId", "subAccountID"),
		Symbol:       getS("symbol"),
		IncomeType:   getS("incomeType", "type"),
		Time:         getI("time", "insertTime", "createTime", "timestamp"),
		Raw:          raw,
	}
}
