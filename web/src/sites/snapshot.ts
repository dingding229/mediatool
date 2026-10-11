export function isSourceCommit(value: string): boolean {
  return /^[a-f0-9]{40}$/.test(value)
}

export function snapshotRequest(ids: string[], sourceCommit = '') {
  if (ids.length < 1 || ids.length > 30 || ids.some(id => !/^[a-z0-9_]{1,80}$/.test(id))) {
    throw new Error('请输入 1–30 个有效站点 ID，仅使用小写字母、数字和下划线。')
  }
  if (new Set(ids).size !== ids.length) throw new Error('站点 ID 不可重复。')
  const commit = sourceCommit.trim().toLowerCase()
  if (commit && !isSourceCommit(commit)) throw new Error('来源提交须为完整的 40 位十六进制 SHA，或留空。')
  return { ids, ...(commit ? { sourceCommit: commit } : {}) }
}
