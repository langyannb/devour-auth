export function formatTs(ts: number): string {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// 金额统一以「分」存储与传输，显示时格式化（AGENTS.md 前端约束）。
export function formatYuan(cents: number): string {
  return (cents / 100).toFixed(2)
}

export function formatDuration(seconds: number): string {
  if (seconds <= 0) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h > 0 ? `${h}h${m}m` : `${m}m`
}

export const statusLabels: Record<string, string> = {
  unused: '未使用',
  active: '使用中',
  expired: '已到期',
  exhausted: '已用尽',
  banned: '已封禁',
  recycled: '已回收',
}

export const kindLabels: Record<string, string> = {
  duration: '时长卡',
  forever: '永久卡',
  count: '次数卡',
  credit: '点数卡',
  timed: '计时卡',
  trial: '试用卡',
}
