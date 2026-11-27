<script setup lang="ts">
import { computed } from 'vue'

interface Item {
  province: string
  logins: number
  ips: number
}

const props = defineProps<{ items: Item[] }>()

// 方块地图：按大致方位摆放，不用真实边界（省份 GeoJSON 来源许可太乱）。
// [列, 行]，从 1 开始。
const layout: Record<string, [number, number]> = {
  黑龙江: [8, 1], 吉林: [8, 2], 辽宁: [7, 3], 内蒙古: [5, 2],
  北京: [6, 3], 天津: [7, 4], 河北: [6, 4], 山西: [5, 4],
  山东: [7, 5], 河南: [6, 5], 江苏: [8, 5], 上海: [9, 6],
  安徽: [7, 6], 浙江: [8, 7], 福建: [7, 8], 江西: [6, 7],
  湖北: [5, 6], 湖南: [5, 7], 广东: [5, 8], 广西: [4, 8],
  海南: [4, 9], 贵州: [4, 7], 云南: [3, 8], 四川: [3, 6],
  重庆: [4, 6], 陕西: [4, 5], 宁夏: [3, 4], 甘肃: [3, 3],
  青海: [2, 4], 西藏: [1, 5], 新疆: [1, 2], 台湾: [9, 8],
  香港: [6, 9], 澳门: [5, 9],
}

const byName = computed(() => new Map(props.items.map((i) => [i.province, i])))
const max = computed(() => Math.max(1, ...props.items.map((i) => i.logins)))

const tiles = computed(() =>
  Object.entries(layout).map(([name, [col, row]]) => {
    const item = byName.value.get(name)
    const logins = item?.logins ?? 0
    const ratio = logins / max.value
    return {
      name,
      col,
      row,
      logins,
      ips: item?.ips ?? 0,
      bg: logins === 0 ? '#f2f5f4' : `rgba(24, 160, 88, ${(0.12 + ratio * 0.78).toFixed(2)})`,
      color: ratio > 0.5 ? '#ffffff' : logins === 0 ? '#b6c0bb' : '#0b3d26',
    }
  }),
)

const other = computed(() => {
  const rest = props.items.filter((i) => !layout[i.province])
  return {
    logins: rest.reduce((n, i) => n + i.logins, 0),
    names: rest.map((i) => i.province).filter((n) => n !== '未知').slice(0, 4),
  }
})
</script>

<template>
  <div class="map">
    <div class="grid" role="img" aria-label="按省份的登录来源分布">
      <div
        v-for="t in tiles"
        :key="t.name"
        class="tile"
        :style="{ gridColumn: t.col, gridRow: t.row, background: t.bg, color: t.color }"
      >
        <span class="name">{{ t.name }}</span>
        <span class="num">{{ t.logins || '' }}</span>
        <title>{{ t.name }}：{{ t.logins }} 次登录 / {{ t.ips }} 个 IP</title>
      </div>
    </div>
    <div class="foot">
      <span class="legend"><i class="lo"></i>少<i class="hi"></i>多</span>
      <span v-if="other.logins > 0" class="other">
        境外/未知 {{ other.logins }} 次<template v-if="other.names.length">（{{ other.names.join('、') }}）</template>
      </span>
      <span v-else class="other">暂无境外来源</span>
    </div>
  </div>
</template>

<style scoped>
.map { width: 100%; overflow-x: auto; }
.grid {
  display: grid;
  grid-template-columns: repeat(9, minmax(36px, 1fr));
  grid-template-rows: repeat(9, minmax(36px, 1fr));
  gap: 4px;
  min-width: 360px;
}
.tile {
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 1px;
  padding: 2px;
  transition: background 0.4s ease;
}
.name { font-size: 10px; line-height: 1.1; letter-spacing: -0.02em; white-space: nowrap; }
.num { font-size: 11px; font-weight: 700; font-variant-numeric: tabular-nums; }
.foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 16px;
  margin-top: 10px;
  font-size: 12px;
  color: #8b93a7;
}
.legend { display: inline-flex; align-items: center; gap: 4px; }
.legend i { width: 14px; height: 8px; border-radius: 2px; display: inline-block; }
.legend i.lo { background: rgba(24, 160, 88, 0.15); }
.legend i.hi { background: rgba(24, 160, 88, 0.9); margin-left: 2px; }
.other { margin-left: auto; }
@media (max-width: 640px) {
  .name { font-size: 9px; }
  .num { font-size: 10px; }
}
</style>
