// Constants for AI model and provider configuration

export interface ComkunProxyModel {
  id: string
  name: string
  provider: string
  desc: string
  icon: string
  price: number  // USD per call
}

export interface AIProviderConfig {
  defaultModel: string
  apiUrl: string
  apiName: string
}

// Get friendly AI model display name（缺字段时勿抛错，避免整页崩溃）
export function getModelDisplayName(rawModelId: string | undefined | null): string {
  if (rawModelId == null || rawModelId === '') return '—'
  const modelId = rawModelId.split('_').pop() || rawModelId
  switch (modelId.toLowerCase()) {
    case 'deepseek':
      return 'DeepSeek'
    case 'qwen':
      return 'Qwen'
    case 'openai':
      return 'OpenAI'
    case 'claude':
      return 'Claude'
    case 'gemini':
      return 'Gemini'
    case 'grok':
      return 'Grok'
    case 'kimi':
      return 'Kimi'
    case 'minimax':
      return 'MiniMax'
    case 'comkun_ai':
      return 'COMKUN-AI'
    case 'comkun_proxy':
      return 'COMKUN-AI 代理模型'
    case 'ai':
      // 兼容旧数据：provider 可能被写成 "ai"
      return 'COMKUN-AI'
    default:
      return modelId.toUpperCase()
  }
}

// 策略市场展示用：按模型标识给一个小图标（避免外部图片依赖）
export function getModelIcon(rawModelId: string | undefined | null): string {
  const id = (rawModelId || '').toLowerCase()
  if (!id) return '🤖'
  if (id.includes('comkun')) return '🧠'
  if (id === 'ai') return '🧠'
  if (id.includes('deepseek')) return '🔥'
  if (id.includes('qwen')) return '⚡'
  if (id.includes('claude')) return '🎯'
  if (id.includes('gpt') || id.includes('openai')) return '🚀'
  if (id.includes('gemini')) return '💎'
  if (id.includes('grok')) return '⚡'
  if (id.includes('kimi') || id.includes('moonshot')) return '🌙'
  if (id.includes('minimax')) return '✨'
  return '🤖'
}

// Extract name part after underscore
export function getShortName(fullName: string | undefined | null): string {
  if (fullName == null || fullName === '') return '—'
  const parts = fullName.split('_')
  return parts.length > 1 ? parts[parts.length - 1] : fullName
}

// Models available through Claw402 (x402 USDC payment protocol)
export const CLAW402_MODELS: ComkunProxyModel[] = [
  { id: 'glm-5', name: 'GLM-5', provider: 'Z.AI', desc: '按实际报价结算', icon: '🧠', price: 0 },
  { id: 'glm-5-turbo', name: 'GLM-5 Turbo', provider: 'Z.AI', desc: '按实际报价结算', icon: '⚡', price: 0 },
  { id: 'deepseek', name: 'DeepSeek V3', provider: 'DeepSeek', desc: '按实际报价结算', icon: '🔥', price: 0 },
  { id: 'deepseek-reasoner', name: 'DeepSeek R1', provider: 'DeepSeek', desc: '按实际报价结算', icon: '🤔', price: 0 },
  { id: 'gpt-5-mini', name: 'GPT-5 Mini', provider: 'OpenAI', desc: '按实际报价结算', icon: '🚀', price: 0 },
  { id: 'qwen-turbo', name: 'Qwen Turbo', provider: 'Alibaba', desc: '按实际报价结算', icon: '⚡', price: 0 },
  { id: 'qwen-flash', name: 'Qwen Flash', provider: 'Alibaba', desc: '按实际报价结算', icon: '⚡', price: 0 },
  { id: 'qwen-plus', name: 'Qwen Plus', provider: 'Alibaba', desc: '按实际报价结算', icon: '✨', price: 0 },
  { id: 'kimi-k2.5', name: 'Kimi K2.5', provider: 'Moonshot', desc: '按实际报价结算', icon: '🌙', price: 0 },
  { id: 'gpt-5.3', name: 'GPT-5.3', provider: 'OpenAI', desc: '按实际报价结算', icon: '💡', price: 0 },
  { id: 'qwen-max', name: 'Qwen Max', provider: 'Alibaba', desc: '按实际报价结算', icon: '🌟', price: 0 },
  { id: 'gemini-3.1-pro', name: 'Gemini 3.1 Pro', provider: 'Google', desc: '按实际报价结算', icon: '💎', price: 0 },
  { id: 'gpt-5.4', name: 'GPT-5.4', provider: 'OpenAI', desc: '按实际报价结算', icon: '⚡', price: 0 },
  { id: 'grok-4.1', name: 'Grok 4.1', provider: 'xAI', desc: '按实际报价结算', icon: '⚡', price: 0 },
  { id: 'claude-opus', name: 'Claude Opus', provider: 'Anthropic', desc: '按实际报价结算', icon: '🎯', price: 0 },
  { id: 'gpt-5.4-pro', name: 'GPT-5.4 Pro', provider: 'OpenAI', desc: '按实际报价结算', icon: '🧠', price: 0 },
]

// AI Provider configuration - default models and API links
export const AI_PROVIDER_CONFIG: Record<string, AIProviderConfig> = {
  deepseek: {
    defaultModel: 'deepseek-v4-flash',
    apiUrl: 'https://platform.deepseek.com/api_keys',
    apiName: 'DeepSeek',
  },
  qwen: {
    defaultModel: 'qwen3.7-max',
    apiUrl: 'https://dashscope.console.aliyun.com/apiKey',
    apiName: 'Alibaba Cloud',
  },
  openai: {
    defaultModel: 'gpt-5.5',
    apiUrl: 'https://platform.openai.com/api-keys',
    apiName: 'OpenAI',
  },
  claude: {
    defaultModel: 'claude-opus-4-8',
    apiUrl: 'https://console.anthropic.com/settings/keys',
    apiName: 'Anthropic',
  },
  gemini: {
    defaultModel: 'gemini-flash-latest',
    apiUrl: 'https://aistudio.google.com/app/apikey',
    apiName: 'Google AI Studio',
  },
  grok: {
    defaultModel: 'grok-4.3-latest',
    apiUrl: 'https://console.x.ai/',
    apiName: 'xAI',
  },
  kimi: {
    defaultModel: 'kimi-k2.6',
    apiUrl: 'https://platform.moonshot.ai/console/api-keys',
    apiName: 'Moonshot',
  },
  minimax: {
    defaultModel: 'MiniMax-M3',
    apiUrl: 'https://platform.minimaxi.com',
    apiName: 'MiniMax',
  },
  claw402: {
    defaultModel: 'glm-5',
    apiUrl: 'https://claw402.ai',
    apiName: 'Claw402',
  },
  comkun_proxy: {
    defaultModel: 'glm-5',
    apiUrl: '',
    apiName: 'COMKUN-AI 代理模型',
  },
  comkun_ai: {
    defaultModel: 'comkun-ai-follow',
    apiUrl: '',
    apiName: 'COMKUN-AI',
  },
}

// Helper function to get exchange display name from exchange ID (UUID)
export function getExchangeDisplayName(exchangeId: string | undefined, exchanges: { id: string; exchange_type?: string; name: string; account_name?: string }[]): string {
  if (!exchangeId) return 'Unknown'
  const exchange = exchanges.find(e => e.id === exchangeId)
  if (!exchange) return exchangeId.substring(0, 8).toUpperCase() + '...' // Show truncated UUID if not found
  const typeName = exchange.exchange_type?.toUpperCase() || exchange.name
  return exchange.account_name ? `${typeName} - ${exchange.account_name}` : typeName
}

// Helper function to check if exchange is a perp-dex type (wallet-based)
export function isPerpDexExchange(exchangeType: string | undefined): boolean {
  if (!exchangeType) return false
  const perpDexTypes = ['hyperliquid', 'lighter', 'aster']
  return perpDexTypes.includes(exchangeType.toLowerCase())
}

// Helper function to get wallet address for perp-dex exchanges
export function getWalletAddress(exchange: { exchange_type?: string; hyperliquidWalletAddr?: string; lighterWalletAddr?: string; asterSigner?: string } | undefined): string | undefined {
  if (!exchange) return undefined
  const type = exchange.exchange_type?.toLowerCase()
  switch (type) {
    case 'hyperliquid':
      return exchange.hyperliquidWalletAddr
    case 'lighter':
      return exchange.lighterWalletAddr
    case 'aster':
      return exchange.asterSigner
    default:
      return undefined
  }
}

// Helper function to truncate wallet address for display
export function truncateAddress(address: string, startLen = 6, endLen = 4): string {
  if (address.length <= startLen + endLen + 3) return address
  return `${address.slice(0, startLen)}...${address.slice(-endLen)}`
}
