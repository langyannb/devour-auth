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

// 复制卡密：剪贴板 API 只在安全上下文可用，局域网 http 访问时退回 execCommand。
// 返回是否真的复制成功，失败不能提示「已复制」。
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 没有焦点或权限时继续走兜底
    }
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  const ok = document.execCommand('copy')
  ta.remove()
  return ok
}

// 到期显示：只有永久卡是「永久」；时长卡的到期时间在首次登录才写入，
// 计时卡没有固定到期日，看剩余时长（design.md 第 424 行表）。
export function formatExpiry(kind: string, expireAt: number, remainingSeconds = 0): string {
  if (kind === 'forever') return '永久'
  if (kind === 'timed') return remainingSeconds > 0 ? `剩余 ${formatDuration(remainingSeconds)}` : '已用完'
  if (expireAt > 0) return formatTs(expireAt)
  if (kind === 'duration' || kind === 'trial') return '未激活'
  return '—'
}

export const kindLabels: Record<string, string> = {
  duration: '时长卡',
  forever: '永久卡',
  count: '次数卡',
  credit: '点数卡',
  timed: '计时卡',
  trial: '试用卡',
}
