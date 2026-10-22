const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'] as const
/** Binary capacities; promote before a rounded display would reach 1000. */
export function formatBytes(input?: number | string | null): string {
  let value: number
  if (typeof input === 'string') {
    const match = input.trim().match(/^(\d+(?:\.\d+)?)\s*(B|[KMGTPE]i?B)?$/i)
    if (!match) return '—'
    const power = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'].indexOf((match[2] || 'B').toUpperCase().replace('I', ''))
    if (power < 0) return '—'
    value = Number(match[1]) * 1024 ** power
  } else value = input ?? NaN
  if (!Number.isFinite(value) || value < 0) return '—'
  let index = 0
  while (Number(value.toFixed(2)) >= 1000 && index < units.length - 1) { value /= 1024; index++ }
  return `${value.toFixed(2)} ${units[index]}`
}
