import { CryptoService } from '../crypto'
import { httpClient } from '../httpClient'
import { formatUserFacingFetchError } from '../userFacingFetchError'

export const API_BASE = '/api'

export { CryptoService, httpClient }

// Helper function to get auth headers
export function getAuthHeaders(): Record<string, string> {
  const token = localStorage.getItem('auth_token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  return headers
}

export async function handleJSONResponse<T>(res: Response): Promise<T> {
  const text = await res.text()
  if (!res.ok) {
    if (import.meta.env.DEV && text.trim()) {
      console.warn('[handleJSONResponse] non-ok', { status: res.status, bodyPreview: text.slice(0, 400) })
    }
    throw new Error(formatUserFacingFetchError(res.status, text || res.statusText))
  }
  if (!text) {
    return {} as T
  }
  return JSON.parse(text) as T
}
