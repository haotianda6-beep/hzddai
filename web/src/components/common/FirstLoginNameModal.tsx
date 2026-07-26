import { useState } from 'react'
import { toast } from 'sonner'
import { api } from '../../lib/api'
import { useAuth } from '../../contexts/AuthContext'

export function FirstLoginNameModal() {
  const { user, token, applyUserProfile } = useAuth()
  const [name, setName] = useState(user?.display_name || '')
  const [saving, setSaving] = useState(false)

  if (!token || !user || user.profile_named || user.is_admin) return null

  const submit = async () => {
    const clean = name.trim()
    if (clean.length < 1 || clean.length > 32) {
      toast.error('请输入 1-32 个字符的名字')
      return
    }
    setSaving(true)
    try {
      const updated = await api.updateDisplayName(clean)
      applyUserProfile({
        display_name: updated.display_name,
        avatar_url: updated.avatar_url,
        profile_named: true,
      })
      toast.success('名字已保存')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-[#d4ff33]/20 bg-nofx-bg-secondary p-6 shadow-2xl">
        <h2 className="text-xl font-bold text-white">先给自己起个名字</h2>
        <p className="mt-2 text-sm leading-relaxed text-zinc-400">
          为了方便策略市场、邀请奖励和后台识别，登录后请使用你自己起的名字，不再使用随机昵称。
        </p>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          maxLength={32}
          placeholder="例如：小李量化"
          className="mt-5 w-full rounded-xl border border-white/10 bg-black/35 px-4 py-3 text-sm text-white outline-none focus:border-[#d4ff33]/50"
        />
        <button
          type="button"
          disabled={saving}
          onClick={() => void submit()}
          className="mt-4 w-full rounded-xl bg-[#d4ff33] py-3 text-sm font-bold text-black disabled:opacity-60"
        >
          {saving ? '保存中…' : '保存并进入'}
        </button>
      </div>
    </div>
  )
}
