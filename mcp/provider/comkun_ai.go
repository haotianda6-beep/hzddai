package provider

import (
	"fmt"
	"time"

	"nofx/mcp"
)

func init() {
	mcp.RegisterProvider(mcp.ProviderComkunAI, func(opts ...mcp.ClientOption) mcp.AIClient {
		return &comkunAIClient{}
	})
}

// comkunAIClient 占位客户端：真实交易由策略 comkun_market_follow + 主广播驱动，不会调用 LLM
type comkunAIClient struct{}

func (c *comkunAIClient) SetAPIKey(apiKey string, customURL string, customModel string) {}

func (c *comkunAIClient) SetTimeout(timeout time.Duration) {}

func (c *comkunAIClient) CallWithMessages(systemPrompt, userPrompt string) (string, error) {
	return "", fmt.Errorf("COMKUN-AI 为平台跟单通道占位模型；请在策略中开启 comkun_market_follow 使用主广播跟单，本接口不会被正常交易周期调用")
}

func (c *comkunAIClient) CallWithRequest(_ *mcp.Request) (string, error) {
	return c.CallWithMessages("", "")
}

func (c *comkunAIClient) CallWithRequestStream(_ *mcp.Request, _ func(string)) (string, error) {
	return "", fmt.Errorf("COMKUN-AI 不支持流式调用")
}

func (c *comkunAIClient) CallWithRequestFull(_ *mcp.Request) (*mcp.LLMResponse, error) {
	return nil, fmt.Errorf("COMKUN-AI 不支持该调用")
}
