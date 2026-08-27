export const formatTime = (value: string) => value
  ? new Date(value).toLocaleString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  : '-'

export const formatDuration = (nanoseconds: number) => `${(nanoseconds / 1_000_000).toFixed(1)} ms`

export const formatScore = (value: number | undefined) => value == null || !Number.isFinite(value)
  ? '—'
  : value.toLocaleString('ja-JP', { maximumFractionDigits: 3 })

export const directoryName = (value: string) => {
  const normalized = value.replaceAll('\\', '/').replace(/\/+$/, '')
  return normalized.split('/').pop() || normalized || '—'
}

export const shortId = (value: string) => value.length > 18 ? value.slice(-18) : value

export const runLabel = (runNumber: number | undefined, id: string) => runNumber && runNumber > 0 ? `#${runNumber}` : shortId(id)

export const statusLabel = (status: string) => ({
  succeeded: '成功',
  running: '実行中',
  queued: '待機中',
  failed: '失敗',
  cancelled: '停止',
  partial: '一部完了',
  wa: 'WA',
  tle: 'TLE'
}[status] ?? status)
