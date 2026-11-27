import { onMounted, onUnmounted, ref } from 'vue'

// 移动端断点与侧栏布局共用，768px 与 Naive UI 的移动栅格一致。
export function useMobile() {
  const isMobile = ref(window.innerWidth < 768)
  const onResize = () => {
    isMobile.value = window.innerWidth < 768
  }
  onMounted(() => window.addEventListener('resize', onResize))
  onUnmounted(() => window.removeEventListener('resize', onResize))
  return isMobile
}
