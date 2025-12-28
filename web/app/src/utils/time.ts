export function fmtTs(ts: number): string {
  if (!ts) return '-'
  const d = new Date(ts * 1000)
  if (Number.isNaN(d.getTime())) return String(ts)
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    d.getFullYear() +
    '-' +
    pad(d.getMonth() + 1) +
    '-' +
    pad(d.getDate()) +
    ' ' +
    pad(d.getHours()) +
    ':' +
    pad(d.getMinutes()) +
    ':' +
    pad(d.getSeconds())
  )
}

export function fromNowSeconds(ts: number): string {
  if (!ts) return '-'
  const now = Math.floor(Date.now() / 1000)
  const diff = now - ts
  if (diff < 0) return 'in future'
  if (diff < 60) return `${diff}s`
  if (diff < 3600) return `${Math.floor(diff / 60)}m`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h`
  return `${Math.floor(diff / 86400)}d`
}
