import { describe, expect, it } from 'vitest'
import { cardKindLabel, durationName, formatExpiry, formatTs, formatYuan } from './utils'

// 金额、时间、到期文案是列表和高频操作的显示基础，坏了很难第一时间发现。
describe('formatYuan', () => {
  it('分转元固定两位小数', () => {
    expect(formatYuan(0)).toBe('0.00')
    expect(formatYuan(300)).toBe('3.00')
    expect(formatYuan(1999)).toBe('19.99')
    expect(formatYuan(77700)).toBe('777.00')
  })
})

describe('formatTs', () => {
  it('0 显示破折号', () => {
    expect(formatTs(0)).toBe('—')
  })
  it('按本地时间格式化', () => {
    const d = new Date(2026, 10, 28, 9, 5, 0)
    expect(formatTs(Math.floor(d.getTime() / 1000))).toBe('2026-11-28 09:05')
  })
})

describe('durationName', () => {
  it('整天的时长按天叫', () => {
    expect(durationName(86400)).toBe('天卡')
    expect(durationName(7 * 86400)).toBe('周卡')
    expect(durationName(30 * 86400)).toBe('月卡')
    expect(durationName(10 * 86400)).toBe('10 天卡')
  })
  it('小时与分钟兜底', () => {
    expect(durationName(3600 * 2)).toBe('2 小时卡')
    expect(durationName(120)).toBe('2 分钟卡')
  })
})

describe('formatExpiry', () => {
  it('永久卡固定显示永久', () => {
    expect(formatExpiry('forever', 0)).toBe('永久')
  })
  it('时长卡未激活不显示 0', () => {
    expect(formatExpiry('duration', 0)).toBe('未激活')
    expect(formatExpiry('duration', 1796000000)).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/)
  })
  it('计时卡按剩余时长', () => {
    expect(formatExpiry('timed', 0, 3600)).toBe('剩余 1h0m')
    expect(formatExpiry('timed', 0, 0)).toBe('已用完')
  })
})

describe('cardKindLabel', () => {
  it('时长卡带天数', () => {
    expect(cardKindLabel('duration', 7 * 86400)).toBe('周卡')
  })
  it('未知类型原样返回', () => {
    expect(cardKindLabel('weird')).toBe('weird')
  })
})
