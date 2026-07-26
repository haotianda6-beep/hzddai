/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Crisp 在线客服 Website ID */
  readonly VITE_CRISP_WEBSITE_ID?: string
  /** Tawk.to Property ID */
  readonly VITE_TAWK_PROPERTY_ID?: string
  /** Tawk.to Widget / Site ID */
  readonly VITE_TAWK_WIDGET_ID?: string
  /** 自定义嵌入客服页（美洽、企点、Chatwoot 等）完整 https URL */
  readonly VITE_LIVECHAT_IFRAME_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
