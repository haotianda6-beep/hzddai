import { useState, useEffect } from 'react'
import { toast } from 'sonner'
import { Plus, Pencil } from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'
import { api } from '../lib/api'
import { ModelConfigModal } from '../components/trader/ModelConfigModal'
import type { AIModel } from '../types'
import { getModelDisplayName } from '../components/trader/model-constants'
import { getModelIcon } from '../components/common/ModelIcons'

export function SettingsPage() {
  const { language } = useLanguage()

  const [configuredModels, setConfiguredModels] = useState<AIModel[]>([])
  const [supportedModels, setSupportedModels] = useState<AIModel[]>([])
  const [showModelModal, setShowModelModal] = useState(false)
  const [editingModel, setEditingModel] = useState<string | null>(null)

  useEffect(() => {
    Promise.all([api.getModelConfigs(), api.getSupportedModels()])
      .then(([configs, supported]) => {
        setConfiguredModels(configs)
        setSupportedModels(supported)
      })
      .catch(() => toast.error('加载 AI 模型失败'))
  }, [])

  const handleSaveModel = async (
    modelId: string,
    apiKey: string,
    customApiUrl?: string,
    customModelName?: string
  ) => {
    try {
      const modelTemplate = supportedModels.find((m) => m.id === modelId)
      const provider = modelTemplate?.provider || modelId
      const existingModel = configuredModels.find(
        (m) => m.id === modelId || m.provider === provider
      )
      const modelToUpdate = existingModel || modelTemplate
      if (!modelToUpdate) {
        toast.error('未找到该模型')
        return
      }

      let updatedModels: AIModel[]
      if (existingModel) {
        const targetId = existingModel.id
        updatedModels = configuredModels.map((m) =>
          m.id === targetId
            ? {
                ...m,
                apiKey,
                customApiUrl: customApiUrl || '',
                customModelName: customModelName || '',
                enabled: true,
              }
            : m
        )
      } else {
        updatedModels = [
          ...configuredModels,
          {
            ...modelToUpdate,
            apiKey,
            customApiUrl: customApiUrl || '',
            customModelName: customModelName || '',
            enabled: true,
          },
        ]
      }

      const request = {
        models: Object.fromEntries(
          updatedModels.map((m) => [
            m.provider,
            {
              enabled: m.enabled,
              api_key: m.apiKey || '',
              custom_api_url: m.customApiUrl || '',
              custom_model_name: m.customModelName || '',
            },
          ])
        ),
      }
      await api.updateModelConfigs(request)
      toast.success('模型配置已保存')
      const refreshed = await api.getModelConfigs()
      setConfiguredModels(refreshed)
      setShowModelModal(false)
      setEditingModel(null)
    } catch {
      toast.error('保存模型配置失败')
    }
  }

  const handleDeleteModel = async (modelId: string) => {
    try {
      const existingModel = configuredModels.find((m) => m.id === modelId)
      const targetId = existingModel?.id || modelId
      const updatedModels = configuredModels.map((m) =>
        m.id === targetId
          ? {
              ...m,
              apiKey: '',
              customApiUrl: '',
              customModelName: '',
              enabled: false,
            }
          : m
      )
      const request = {
        models: Object.fromEntries(
          updatedModels.map((m) => [
            m.provider,
            {
              enabled: m.enabled,
              api_key: m.apiKey || '',
              custom_api_url: m.customApiUrl || '',
              custom_model_name: m.customModelName || '',
            },
          ])
        ),
      }
      await api.updateModelConfigs(request)
      const refreshed = await api.getModelConfigs()
      setConfiguredModels(refreshed)
      setShowModelModal(false)
      setEditingModel(null)
      toast.success('已移除该模型配置')
    } catch {
      toast.error('移除模型配置失败')
    }
  }

  return (
    <div
      className="min-h-screen px-3 pb-12 pt-16 sm:px-4 sm:pt-20"
      style={{ background: '#0b0b0b' }}
    >
      <div className="max-w-2xl mx-auto">
        <h1 className="text-xl font-bold text-white mb-1">设置</h1>
        <p className="text-sm text-zinc-500 mb-6">
          仅管理 AI 模型与 API；交易所账户请在部署向导或看板相关入口配置。
        </p>

        <div className="rounded-2xl border border-zinc-800/80 bg-zinc-900/60 p-4 backdrop-blur-xl sm:p-6">
          <div className="space-y-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-sm text-zinc-400">
                已配置 {configuredModels.length} 个模型
              </p>
              <button
                type="button"
                onClick={() => {
                  setEditingModel(null)
                  setShowModelModal(true)
                }}
                className="flex h-10 w-full items-center justify-center gap-1.5 rounded-lg bg-nofx-gold/10 px-3 py-1.5 text-xs font-medium text-nofx-gold transition-colors hover:bg-nofx-gold/20 sm:h-auto sm:w-auto"
              >
                <Plus size={14} />
                添加模型
              </button>
            </div>

            {configuredModels.length === 0 ? (
              <div className="text-center py-8 text-zinc-600 text-sm">
                尚未配置任何 AI 模型，请点击右上角「添加模型」。
              </div>
            ) : (
              <div className="space-y-2">
                {configuredModels.map((model) => (
                  <button
                    type="button"
                    key={model.id}
                    onClick={() => {
                      setEditingModel(model.id)
                      setShowModelModal(true)
                    }}
                    className="group flex w-full items-center justify-between gap-3 rounded-xl border border-zinc-700/50 bg-zinc-800/50 px-3 py-3 transition-colors hover:bg-zinc-800 sm:px-4"
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-zinc-700">
                        {getModelIcon(model.provider || model.id, {
                          width: 18,
                          height: 18,
                          className: 'opacity-90',
                        }) || (
                          <span className="text-sm font-bold text-zinc-300" aria-hidden>
                            {getModelDisplayName(model.provider || model.id).slice(0, 1)}
                          </span>
                        )}
                      </div>
                      <div className="min-w-0 text-left">
                        <p className="truncate text-sm font-medium text-white">
                          {getModelDisplayName(model.provider || model.id)} AI
                        </p>
                        <p className="truncate text-xs text-zinc-500">{model.provider || model.id}</p>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <span
                        className={`text-xs px-2 py-0.5 rounded-full ${model.enabled ? 'bg-emerald-500/10 text-emerald-400' : 'bg-zinc-700 text-zinc-500'}`}
                      >
                        {model.enabled ? '已启用' : '未启用'}
                      </span>
                      <Pencil
                        size={14}
                        className="text-zinc-600 group-hover:text-zinc-400 transition-colors"
                      />
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {showModelModal && (
        <ModelConfigModal
          allModels={supportedModels}
          configuredModels={configuredModels}
          editingModelId={editingModel}
          onSave={handleSaveModel}
          onDelete={handleDeleteModel}
          onClose={() => {
            setShowModelModal(false)
            setEditingModel(null)
          }}
          language={language}
        />
      )}
    </div>
  )
}
