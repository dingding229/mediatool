export interface LogRecord { time?: string; level?: string; msg?: string; source?: string | { file?: string; line?: number }; module?: string; [key: string]: unknown }
export function formatLogTime(value: unknown): string {
  if (typeof value !== 'string') return '时间未知'
  const date = new Date(value.replace(/(\.\d{3})\d+(?=Z|[+-])/, '$1'))
  if (!Number.isFinite(date.getTime())) return '时间未知'
  const pad = (n: number) => String(n).padStart(2,'0')
  return `${date.getFullYear()}-${pad(date.getMonth()+1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}
export function logLevel(value: unknown) {
  const level = String(value ?? '').toLowerCase()
  return level === 'warning' ? 'warn' : ['debug','info','warn','error'].includes(level) ? level : 'unknown'
}
export function logSource(row: LogRecord): string {
  if (typeof row.source === 'string') return row.source
  if (row.source?.file) return row.source.file + (row.source.line ? `:${row.source.line}` : '')
  return ''
}
export function logFields(row: LogRecord): Record<string, unknown> {
  return Object.fromEntries(Object.entries(row).filter(([key]) => !['time','level','msg','source','module'].includes(key)))
}
export function inlineLogFields(row: LogRecord): string {
  return Object.entries(logFields(row)).filter(([key])=>!['requestId','request_id','traceId','trace_id','span','spanId','span_id'].includes(key)).map(([key,value]) => {
    const text = typeof value === 'string' ? value : JSON.stringify(value)
    return `${key}=${String(text).slice(0,140)}`
  }).join('  ')
}
export function logText(rows: LogRecord[], clean: boolean): string {
  return rows.map(row => [formatLogTime(row.time),logLevel(row.level),String(row.msg ?? ''),clean ? '' : Object.entries(logFields(row)).map(([key,value])=>`${key}=${typeof value === 'string' ? value : JSON.stringify(value)}`).join('  '),clean ? '' : logSource(row)].filter(Boolean).join('  ')).join('\n')
}
