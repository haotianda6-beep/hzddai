import { ROUTES } from '../router/paths'

export type UserMode = 'beginner' | 'advanced'

const USER_MODE_KEY = 'nofx_user_mode'
const BEGINNER_WALLET_ADDRESS_KEY = 'nofx_beginner_wallet_address'
const BEGINNER_ONBOARDING_COMPLETED_KEY = 'nofx_beginner_onboarding_completed'

/** 产品已统一为新手模式；本地若曾存老手也按新手处理 */
export function getUserMode(): UserMode {
  return 'beginner'
}

export function setUserMode(_mode?: UserMode) {
  localStorage.setItem(USER_MODE_KEY, 'beginner')
}

/** 登录/注册成功后的默认落地页（不再弹出新手钱包准备页） */
export function getPostAuthPath(_mode?: UserMode | null): string {
  return ROUTES.traders
}

export function setBeginnerWalletAddress(address: string) {
  localStorage.setItem(BEGINNER_WALLET_ADDRESS_KEY, address)
}

export function getBeginnerWalletAddress(): string | null {
  return localStorage.getItem(BEGINNER_WALLET_ADDRESS_KEY)
}

export function hasCompletedBeginnerOnboarding(): boolean {
  return localStorage.getItem(BEGINNER_ONBOARDING_COMPLETED_KEY) === 'true'
}

export function markBeginnerOnboardingCompleted() {
  localStorage.setItem(BEGINNER_ONBOARDING_COMPLETED_KEY, 'true')
}
