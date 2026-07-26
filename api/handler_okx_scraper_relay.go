package api

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleOkxScraperRelay forwards browser Tampermonkey POST body to local okx_dom_scraper.
// HTTPS 中转：避免 OKX 页面 Mixed Content + Tampermonkey background shutdown 直连 HTTP 失败。
func (s *Server) handleOkxScraperRelay(c *gin.Context) {
	if c.Request.Method == http.MethodOptions {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-OKX-Scraper-Route, X-OKX-Scraper-Token")
		c.Status(http.StatusNoContent)
		return
	}

	if token := strings.TrimSpace(os.Getenv("OKX_SCRAPER_RELAY_TOKEN")); token != "" {
		got := strings.TrimSpace(c.GetHeader("X-OKX-Scraper-Token"))
		if got == "" {
			got = strings.TrimSpace(c.Query("token"))
		}
		if got != token {
			c.Header("Access-Control-Allow-Origin", "*")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		SafeBadRequest(c, "empty body")
		return
	}

	port, ok := resolveOkxScraperRelayPort(c)
	if !ok {
		c.Header("Access-Control-Allow-Origin", "*")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid okx scraper route"})
		return
	}
	rememberOkxMarketHistoryUniqueNameFromRelay(port, body)
	host := strings.TrimSpace(os.Getenv("OKX_SCRAPER_HOST"))
	if host == "" {
		host = "host.docker.internal"
	}
	target := "http://" + host + ":" + strconv.Itoa(port) + "/data"

	ctx := c.Request.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		SafeInternalError(c, "relay request", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "okx scraper unreachable",
			"detail": err.Error(),
			"target": target,
			"hint":   "ensure okx-scraper-01.service is running",
		})
		return
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)
	c.Header("Access-Control-Allow-Origin", "*")
	c.Data(resp.StatusCode, "application/json", out)
}

func resolveOkxScraperRelayPort(c *gin.Context) (int, bool) {
	allowed := okxScraperAllowedPorts()
	defaultPort := 18765
	if raw := strings.TrimSpace(os.Getenv("OKX_SCRAPER_PORT")); raw != "" {
		if p, err := strconv.Atoi(raw); err == nil {
			defaultPort = p
		}
	}

	rawRoute := strings.TrimSpace(c.Query("route"))
	if rawRoute == "" {
		rawRoute = strings.TrimSpace(c.GetHeader("X-OKX-Scraper-Route"))
	}
	if rawRoute != "" {
		switch strings.TrimLeft(strings.ToLower(rawRoute), "0") {
		case "1":
			return 18765, allowed[18765]
		case "2":
			return 18766, allowed[18766]
		case "3":
			return 18767, allowed[18767]
		case "4":
			return 18768, allowed[18768]
		case "5":
			return 18769, allowed[18769]
		case "6":
			return 18770, allowed[18770]
		case "7":
			return 18771, allowed[18771]
		case "8":
			return 18772, allowed[18772]
		}
		return 0, false
	}

	rawPort := strings.TrimSpace(c.Query("port"))
	if rawPort == "" {
		rawPort = strings.TrimSpace(c.GetHeader("X-OKX-Scraper-Port"))
	}
	if rawPort != "" {
		p, err := strconv.Atoi(rawPort)
		if err != nil {
			return 0, false
		}
		return p, allowed[p]
	}
	return defaultPort, allowed[defaultPort]
}

func okxScraperAllowedPorts() map[int]bool {
	raw := strings.TrimSpace(os.Getenv("OKX_SCRAPER_ALLOWED_PORTS"))
	if raw == "" {
		raw = "18765,18766,18767,18768,18769,18770,18771,18772"
	}
	out := make(map[int]bool)
	for _, part := range strings.Split(raw, ",") {
		p, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || p <= 0 || p >= 65536 {
			continue
		}
		out[p] = true
	}
	return out
}
