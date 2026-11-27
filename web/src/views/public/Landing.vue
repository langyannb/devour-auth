<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

const host = location.hostname
const isLocal = host === 'localhost' || host === '127.0.0.1' || host === ''
const adminURL = isLocal ? '/login?role=admin' : 'https://admin.namikao.com/login'
const agentURL = isLocal ? '/login?role=agent' : 'https://agent.namikao.com/login'

const features = [
  {
    icon: 'shield',
    title: '授权判定在服务端',
    body: '卡密、设备指纹、权限、额度全部由服务端裁决。客户端被改也只能被拒绝，不能伪造通过。',
  },
  {
    icon: 'layers',
    title: '六种卡类型',
    body: '时长、永久、次数、点数、计时、试用，各自独立的到期与耗尽规则，出卡时冻结快照。',
  },
  {
    icon: 'device',
    title: '设备绑定与换绑',
    body: '一台设备一个指纹，超出上限直接拒绝。免费换绑次数、冷却天数、超额扣时都由策略定义。',
  },
  {
    icon: 'chain',
    title: '审计不可改写',
    body: '每次管理操作写入 SHA-256 哈希链，数据库触发器禁止 UPDATE / DELETE，每日用 Ed25519 对链尾签名。',
  },
  {
    icon: 'users',
    title: '代理分销',
    body: '两级代理、折扣基点、月度额度、上级返点与账单，出卡与记账在同一事务里完成。',
  },
  {
    icon: 'code',
    title: '开放接口',
    body: 'HMAC 签名的开放 API，可对接发货系统。时间窗口 5 分钟、nonce 去重、权限按密钥隔离。',
  },
]

const steps = [
  { n: '01', t: '出卡', d: '后台或代理端批量生成，卡密不连续、不带业务信息。' },
  { n: '02', t: '客户端登录', d: '卡密 + 设备指纹换 access / refresh，令牌库内只存哈希。' },
  { n: '03', t: '持续校验', d: '心跳与消耗按策略扣减，暂停、吊销、封禁即时生效。' },
]

const facts = [
  { v: 6, suffix: '', label: '种卡类型' },
  { v: 403, suffix: '', label: '交叉访问一律拒绝' },
  { v: 15, suffix: 'min', label: 'access 令牌有效期' },
  { v: 0, suffix: '', label: '客户端可信任的判定' },
]

const factValues = ref<number[]>(facts.map(() => 0))
const scrolled = ref(false)
let observer: IntersectionObserver | null = null
let raf = 0

function countUp() {
  const start = performance.now()
  const duration = 900
  const tick = (now: number) => {
    const p = Math.min(1, (now - start) / duration)
    const eased = 1 - Math.pow(1 - p, 3)
    factValues.value = facts.map((f) => Math.round(f.v * eased))
    if (p < 1) raf = requestAnimationFrame(tick)
  }
  raf = requestAnimationFrame(tick)
}

function onScroll() {
  scrolled.value = window.scrollY > 8
}

onMounted(() => {
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (reduced) {
    factValues.value = facts.map((f) => f.v)
    document.querySelectorAll('.reveal').forEach((el) => el.classList.add('in'))
  } else {
    countUp()
    observer = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) {
            e.target.classList.add('in')
            observer?.unobserve(e.target)
          }
        }
      },
      { threshold: 0.15 },
    )
    document.querySelectorAll('.reveal').forEach((el) => observer?.observe(el))
  }
  window.addEventListener('scroll', onScroll, { passive: true })
  onScroll()
})

onUnmounted(() => {
  observer?.disconnect()
  cancelAnimationFrame(raf)
  window.removeEventListener('scroll', onScroll)
})
</script>

<template>
  <div class="landing">
    <header class="nav" :class="{ scrolled }">
      <a class="brand" href="/">
        <span class="brand-mark">D</span>
        <span class="brand-name">Devour<em>Auth</em></span>
      </a>
      <nav class="links">
        <a href="#features">能力</a>
        <a href="#flow">接入</a>
        <a href="/query">查卡密</a>
        <a :href="adminURL" class="btn sm">进入后台</a>
      </nav>
    </header>

    <section class="hero">
      <div class="grid-layer"></div>
      <div class="glow a"></div>
      <div class="glow b"></div>
      <div class="glow c"></div>

      <div class="hero-inner">
        <div class="hero-copy">
          <p class="eyebrow"><span class="dot-live"></span>自托管 · 单二进制 · 判定在服务端</p>
          <h1>客户端只能听结果<br /><span class="grad">判定权不离开你的服务器</span></h1>
          <p class="lede">
            卡密、设备绑定、功能权限、代理分销，一套跑在自己服务器上的授权系统。
            令牌只存哈希，管理动作全部进哈希链，吊销即时生效。
          </p>
          <div class="cta">
            <a class="btn primary" href="/query">查询卡密状态</a>
            <a class="btn ghost" :href="agentURL">代理登录</a>
          </div>
          <ul class="facts">
            <li v-for="(f, i) in facts" :key="f.label">
              <strong>{{ factValues[i] }}<em v-if="f.suffix">{{ f.suffix }}</em></strong>
              <span>{{ f.label }}</span>
            </li>
          </ul>
        </div>

        <div class="hero-visual reveal">
          <div class="glass">
            <div class="glass-head">
              <span class="traffic"><i></i><i></i><i></i></span>
              <span class="glass-title">authorize</span>
              <span class="pill-live"><i></i>服务端判定</span>
            </div>
            <pre class="code"><span class="ln"><b>POST</b> /v1/authorize</span><span class="ln dim">{ "kami": "YY94-****-CFD4", "device": "9f2c…a1" }</span><span class="ln ok">← 200 { "ok": true, "expire_at": "2026-12-28" }</span><span class="ln ok">← 403 { "ok": false, "code": "DEVICE_LIMIT" }</span><span class="ln caret">检查设备指纹与授权快照<span class="caret-bar"></span></span></pre>
            <div class="glass-foot">
              <span>access 15min</span>
              <span>refresh 哈希入库</span>
              <span>审计链已锚定</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section id="features" class="section">
      <div class="sec-head reveal">
        <h2>它解决什么</h2>
        <p class="sub">不是把密码学塞进客户端，而是让客户端没有决定权。</p>
      </div>
      <div class="grid">
        <article v-for="(f, i) in features" :key="f.title" class="card reveal" :style="{ transitionDelay: i * 60 + 'ms' }">
          <span class="icon" aria-hidden="true">
            <svg v-if="f.icon === 'shield'" viewBox="0 0 24 24"><path d="M12 3l7 3v6c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6l7-3z" /><path d="M9 12l2 2 4-4" /></svg>
            <svg v-else-if="f.icon === 'layers'" viewBox="0 0 24 24"><path d="M12 3l9 5-9 5-9-5 9-5z" /><path d="M3 13l9 5 9-5" /></svg>
            <svg v-else-if="f.icon === 'device'" viewBox="0 0 24 24"><rect x="7" y="3" width="10" height="18" rx="2" /><path d="M11 18h2" /></svg>
            <svg v-else-if="f.icon === 'chain'" viewBox="0 0 24 24"><path d="M10 13a5 5 0 007 0l2-2a5 5 0 00-7-7l-1 1" /><path d="M14 11a5 5 0 00-7 0l-2 2a5 5 0 007 7l1-1" /></svg>
            <svg v-else-if="f.icon === 'users'" viewBox="0 0 24 24"><circle cx="9" cy="8" r="3" /><path d="M3 20c0-3.3 2.7-5 6-5s6 1.7 6 5" /><path d="M16 5.5a3 3 0 010 5.5M18 20c0-2.4-.8-4-2-4.8" /></svg>
            <svg v-else viewBox="0 0 24 24"><path d="M8 6l-5 6 5 6M16 6l5 6-5 6" /></svg>
          </span>
          <h3>{{ f.title }}</h3>
          <p>{{ f.body }}</p>
        </article>
      </div>
    </section>

    <section id="flow" class="section alt">
      <div class="sec-head reveal">
        <h2>接入只有三步</h2>
        <p class="sub">从出卡到吊销，客户端拿到的始终只是一张有时限的令牌。</p>
      </div>
      <div class="steps">
        <div v-for="(s, i) in steps" :key="s.n" class="step reveal" :style="{ transitionDelay: i * 90 + 'ms' }">
          <span class="num">{{ s.n }}</span>
          <h3>{{ s.t }}</h3>
          <p>{{ s.d }}</p>
        </div>
      </div>
    </section>

    <section class="section">
      <div class="strip reveal">
        <div class="strip-text">
          <h2>已经在跑</h2>
          <p>服务端、管理端、代理端均已上线，审计链每日自动锚定，数据库每小时加密备份。</p>
        </div>
        <div class="strip-actions">
          <a class="btn primary" :href="adminURL">管理端登录</a>
          <a class="btn" href="/query">查卡密</a>
        </div>
      </div>
    </section>

    <footer class="footer">
      <span class="foot-brand"><span class="brand-mark sm">D</span>Devour Auth</span>
      <span class="dot">·</span>
      <a :href="adminURL">管理端</a>
      <a :href="agentURL">代理端</a>
      <a href="/query">卡密查询</a>
      <span class="copy">© {{ new Date().getFullYear() }}</span>
    </footer>
  </div>
</template>

<style scoped>
.landing {
  min-height: 100vh;
  background: #f7f9f8;
  color: #12151c;
}

/* ---------- nav ---------- */
.nav {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 32px;
  background: transparent;
  border-bottom: 1px solid transparent;
  transition: background 0.25s ease, border-color 0.25s ease, backdrop-filter 0.25s ease;
}
.nav.scrolled {
  background: rgba(9, 15, 21, 0.94);
  backdrop-filter: blur(14px) saturate(1.2);
  border-bottom-color: rgba(255, 255, 255, 0.08);
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  text-decoration: none;
  color: #fff;
}
.brand-mark {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  font-size: 14px;
  font-weight: 800;
  color: #04140c;
  background: linear-gradient(135deg, #4ade9b, #18a058);
}
.brand-mark.sm { width: 22px; height: 22px; border-radius: 6px; font-size: 11px; }
.brand-name { font-weight: 800; font-size: 16px; letter-spacing: -0.02em; }
.brand-name em { font-style: normal; color: #4ade9b; }
.links { display: flex; align-items: center; gap: 22px; font-size: 14px; }
.links a { color: rgba(233, 240, 236, 0.72); text-decoration: none; transition: color 0.15s ease; }
.links a:hover { color: #fff; }

/* ---------- hero ---------- */
.hero {
  position: relative;
  overflow: hidden;
  margin-top: -57px;
  padding: 128px 32px 88px;
  background: radial-gradient(120% 90% at 50% -10%, #0f2430 0%, #0a121a 55%, #06090d 100%);
  color: #eef4f0;
  isolation: isolate;
}
.grid-layer {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(rgba(255, 255, 255, 0.09) 1px, transparent 1px);
  background-size: 26px 26px;
  mask-image: radial-gradient(90% 70% at 50% 20%, #000 30%, transparent 75%);
  opacity: 0.5;
}
.glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  pointer-events: none;
  will-change: transform;
}
.glow.a {
  width: 460px;
  height: 460px;
  background: #18a058;
  top: -180px;
  left: 4%;
  opacity: 0.45;
  animation: drift-a 18s ease-in-out infinite alternate;
}
.glow.b {
  width: 380px;
  height: 380px;
  background: #06b6d4;
  top: -120px;
  right: 2%;
  opacity: 0.28;
  animation: drift-b 22s ease-in-out infinite alternate;
}
.glow.c {
  width: 300px;
  height: 300px;
  background: #4ade9b;
  bottom: -180px;
  left: 38%;
  opacity: 0.16;
  animation: drift-a 26s ease-in-out infinite alternate-reverse;
}
@keyframes drift-a {
  to { transform: translate3d(40px, 30px, 0) scale(1.12); }
}
@keyframes drift-b {
  to { transform: translate3d(-50px, 24px, 0) scale(1.08); }
}

.hero-inner {
  position: relative;
  max-width: 1140px;
  margin: 0 auto;
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 0.95fr);
  gap: 48px;
  align-items: center;
}
.hero-copy { animation: rise 0.7s cubic-bezier(0.16, 1, 0.3, 1) both; }
@keyframes rise {
  from { opacity: 0; transform: translateY(14px); }
}
.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 18px;
  padding: 6px 12px;
  border-radius: 99px;
  font-size: 12px;
  letter-spacing: 0.06em;
  color: #a7f3cf;
  background: rgba(24, 160, 88, 0.14);
  border: 1px solid rgba(74, 222, 155, 0.28);
}
.dot-live {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #4ade9b;
  box-shadow: 0 0 0 0 rgba(74, 222, 155, 0.7);
  animation: live 2.2s ease-out infinite;
}
@keyframes live {
  70% { box-shadow: 0 0 0 7px rgba(74, 222, 155, 0); }
  100% { box-shadow: 0 0 0 0 rgba(74, 222, 155, 0); }
}
.hero h1 {
  margin: 0 0 20px;
  font-size: clamp(30px, 4.4vw, 50px);
  line-height: 1.16;
  letter-spacing: -0.035em;
}
.grad {
  background: linear-gradient(100deg, #eafff4 10%, #4ade9b 45%, #38bdf8 90%);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.lede {
  margin: 0 0 30px;
  max-width: 540px;
  font-size: 16px;
  line-height: 1.8;
  color: #a9b8b2;
}
.cta { display: flex; gap: 12px; flex-wrap: wrap; }
.facts {
  display: flex;
  flex-wrap: wrap;
  gap: 30px;
  margin: 42px 0 0;
  padding: 0;
  list-style: none;
  font-size: 12px;
  color: #7f918a;
}
.facts strong {
  display: block;
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -0.02em;
  color: #fff;
  font-variant-numeric: tabular-nums;
}
.facts em { font-style: normal; font-size: 12px; color: #4ade9b; margin-left: 1px; }

/* 视觉卡 */
.hero-visual { animation: rise 0.9s 0.12s cubic-bezier(0.16, 1, 0.3, 1) both; }
.glass {
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.07), rgba(255, 255, 255, 0.025));
  box-shadow: 0 40px 80px -50px rgba(0, 0, 0, 0.9), inset 0 1px 0 rgba(255, 255, 255, 0.06);
  backdrop-filter: blur(10px);
  overflow: hidden;
  animation: bob 7s ease-in-out infinite;
}
@keyframes bob {
  50% { transform: translateY(-8px); }
}
.glass-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 11px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}
.traffic { display: inline-flex; gap: 5px; }
.traffic i { width: 8px; height: 8px; border-radius: 50%; background: rgba(255, 255, 255, 0.18); }
.glass-title { font-size: 12px; color: #90a49c; font-family: ui-monospace, monospace; }
.pill-live {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: #a7f3cf;
  padding: 3px 8px;
  border-radius: 99px;
  background: rgba(24, 160, 88, 0.18);
}
.pill-live i { width: 5px; height: 5px; border-radius: 50%; background: #4ade9b; animation: live 2.2s ease-out infinite; }
.code {
  margin: 0;
  padding: 16px 16px 18px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  line-height: 2;
  color: #d7e4de;
  white-space: pre-wrap;
  word-break: break-word;
}
.code .ln { display: block; opacity: 0; animation: line-in 0.4s ease forwards; }
.code .ln:nth-child(1) { animation-delay: 0.35s; }
.code .ln:nth-child(2) { animation-delay: 0.6s; }
.code .ln:nth-child(3) { animation-delay: 0.85s; }
.code .ln:nth-child(4) { animation-delay: 1.1s; }
.code .ln:nth-child(5) { animation-delay: 1.35s; }
@keyframes line-in {
  from { opacity: 0; transform: translateX(-4px); }
  to { opacity: 1; transform: none; }
}
.code b { color: #7dd3fc; font-weight: 700; }
.code .dim { color: #8fa39b; }
.code .ok { color: #6ee7b7; }
.code .caret { color: #8fa39b; }
.caret-bar {
  display: inline-block;
  width: 7px;
  height: 13px;
  margin-left: 3px;
  vertical-align: -2px;
  background: #4ade9b;
  animation: blink 1.1s steps(2, start) infinite;
}
@keyframes blink { 50% { opacity: 0; } }
.glass-foot {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  padding: 10px 16px 12px;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
  font-size: 11px;
  color: #7f918a;
}

/* ---------- buttons ---------- */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 11px 20px;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
  border: 1px solid transparent;
  transition: transform 0.15s ease, box-shadow 0.2s ease, background 0.2s ease, border-color 0.2s ease;
}
.btn:hover { transform: translateY(-2px); }
.btn.primary {
  background: linear-gradient(135deg, #1fb46a, #148c4f);
  color: #fff;
  box-shadow: 0 14px 30px -14px rgba(24, 160, 88, 0.9);
}
.btn.primary:hover { box-shadow: 0 18px 36px -14px rgba(24, 160, 88, 1); }
.btn.ghost {
  border-color: rgba(255, 255, 255, 0.2);
  color: #e9f0ec;
}
.btn.ghost:hover { background: rgba(255, 255, 255, 0.07); }
.btn.sm { padding: 8px 15px; font-size: 13px; background: #fff; color: #0b3d26; }
.btn.sm:hover { background: #eafaf1; }

/* ---------- sections ---------- */
.section { max-width: 1120px; margin: 0 auto; padding: 84px 32px; }
.section.alt {
  max-width: none;
  background: #fff;
  border-top: 1px solid #e8eeeb;
  border-bottom: 1px solid #e8eeeb;
}
.section.alt .sec-head,
.section.alt .steps { max-width: 1056px; margin-left: auto; margin-right: auto; }
.sec-head { margin-bottom: 34px; }
.section h2 { margin: 0 0 10px; font-size: 28px; letter-spacing: -0.02em; }
.sub { margin: 0; color: #5b6478; font-size: 15px; }

.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; }
.card {
  padding: 24px;
  border-radius: 14px;
  background: #fff;
  border: 1px solid #e8eeeb;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.03);
  transition: transform 0.2s ease, box-shadow 0.25s ease, border-color 0.2s ease;
}
.card:hover {
  transform: translateY(-3px);
  border-color: #bfe3cf;
  box-shadow: 0 22px 40px -30px rgba(16, 24, 40, 0.55);
}
.icon {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  margin-bottom: 14px;
  color: #148c4f;
  background: #eafaf1;
  transition: background 0.2s ease, transform 0.2s ease;
}
.card:hover .icon { background: #d8f5e5; transform: scale(1.06); }
.icon svg {
  width: 20px;
  height: 20px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.7;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.card h3 { margin: 0 0 8px; font-size: 16px; }
.card p { margin: 0; font-size: 14px; line-height: 1.75; color: #5b6478; }

.steps { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; }
.step {
  position: relative;
  padding: 22px 22px 24px;
  border-radius: 14px;
  background: #f9fbfa;
  border: 1px solid #e8eeeb;
}
.step .num {
  display: inline-block;
  font-family: ui-monospace, monospace;
  font-size: 12px;
  font-weight: 700;
  color: #148c4f;
  background: #eafaf1;
  border-radius: 6px;
  padding: 3px 7px;
}
.step h3 { margin: 12px 0 6px; font-size: 17px; }
.step p { margin: 0; font-size: 14px; line-height: 1.7; color: #5b6478; }

.strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
  padding: 36px 38px;
  border-radius: 20px;
  background: linear-gradient(115deg, #0d2b1d 0%, #123a28 55%, #0b2f3a 100%);
  color: #eaf7f0;
  position: relative;
  overflow: hidden;
}
.strip::after {
  content: '';
  position: absolute;
  top: 0;
  left: -40%;
  width: 40%;
  height: 100%;
  background: linear-gradient(100deg, transparent, rgba(110, 231, 183, 0.16), transparent);
  animation: sweep 6s ease-in-out infinite;
}
@keyframes sweep {
  0% { left: -45%; }
  55%, 100% { left: 115%; }
}
.strip-text h2 { margin: 0 0 8px; font-size: 24px; color: #fff; }
.strip-text p { margin: 0; color: #a9c7b8; font-size: 14px; }
.strip-actions { display: flex; gap: 10px; flex-wrap: wrap; position: relative; z-index: 1; }
.strip .btn { background: #fff; color: #0b3d26; }
.strip .btn.primary { background: linear-gradient(135deg, #2fd184, #18a058); color: #04140c; }
.strip .btn.primary:hover { box-shadow: 0 18px 36px -16px rgba(47, 209, 132, 0.9); }

.footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  justify-content: center;
  padding: 30px 24px 44px;
  font-size: 13px;
  color: #7b8794;
}
.footer a { color: #4a5568; text-decoration: none; transition: color 0.15s ease; }
.footer a:hover { color: #12151c; }
.foot-brand { display: inline-flex; align-items: center; gap: 8px; font-weight: 600; color: #12151c; }
.dot { opacity: 0.4; }
.copy { color: #9aa3b2; }

/* ---------- reveal ---------- */
.reveal {
  opacity: 0;
  transform: translateY(18px);
  transition: opacity 0.6s cubic-bezier(0.16, 1, 0.3, 1), transform 0.6s cubic-bezier(0.16, 1, 0.3, 1);
}
.reveal.in { opacity: 1; transform: none; }

@media (max-width: 900px) {
  .hero-inner { grid-template-columns: minmax(0, 1fr); gap: 40px; }
  .hero { padding: 112px 22px 72px; }
  .hero-visual { order: 2; }
}
@media (max-width: 640px) {
  .nav { padding: 12px 16px; }
  .links { gap: 12px; font-size: 13px; }
  .links a:not(.btn) { display: none; }
  .section { padding: 60px 18px; }
  .facts { gap: 22px; }
  .strip { padding: 26px; border-radius: 16px; }
  .strip-actions { width: 100%; }
  .strip-actions .btn { flex: 1; }
  .code { font-size: 11.5px; }
}
@media (prefers-reduced-motion: reduce) {
  .glow, .glass, .dot-live, .pill-live i, .caret-bar, .strip::after { animation: none; }
  .hero-copy, .hero-visual, .code .ln { animation: none; opacity: 1; }
  .reveal { opacity: 1; transform: none; transition: none; }
  .btn:hover, .card:hover, .card:hover .icon { transform: none; }
  .nav { transition: none; }
}
</style>
