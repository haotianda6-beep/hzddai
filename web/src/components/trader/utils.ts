// 获取友好的AI模型名称
export function getModelDisplayName(modelId: string | undefined | null): string {
  if (modelId == null || modelId === '') return '—'
  switch (modelId.toLowerCase()) {
    case 'deepseek':
      return 'DeepSeek'
    case 'qwen':
      return 'Qwen'
    case 'claude':
      return 'Claude'
    default:
      return modelId.toUpperCase()
  }
}

// 提取下划线后面的名称部分
export function getShortName(fullName: string | undefined | null): string {
  if (fullName == null || fullName === '') return '—'
  const parts = fullName.split('_')
  return parts.length > 1 ? parts[parts.length - 1] : fullName
}
