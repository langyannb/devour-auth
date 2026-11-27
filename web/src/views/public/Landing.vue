<script setup lang="ts">
const host = location.hostname
const isLocal = host === 'localhost' || host === '127.0.0.1' || host === ''
const adminURL = isLocal ? '/login?role=admin' : 'https://admin.namikao.com/login'
const agentURL = isLocal ? '/login?role=agent' : 'https://agent.namikao.com/login'

const features = [
  {
    title: '授权判定在服务端',
    body: '卡密、设备指纹、权限、额度全部由服务端裁决。客户端被改也只能被拒绝，不能伪造通过。',
  },
  {
    title: '六种卡类型',
    body: '时长、永久、次数、点数、计时、试用，各自独立的到期与耗尽规则，出卡时冻结快照。',
  },
  {
    title: '设备绑定与换绑',
    body: '一台设备一个指纹，超出上限直接拒绝。免费换绑次数、冷却天数、超额扣时都由策略定义。',
  },
  {
    title: '审计不可改写',
    body: '每次管理操作写入 SHA-256 哈希链，数据库触发器禁止 UPDATE / DELETE，每日用 Ed25519 对链尾签名。',
  },
  {
    title: '代理分销',
    body: '两级代理、折扣基点、月度额度、上级返点与账单，出卡与记账在同一事务里完成。',
  },
  {
    title: '开放接口',
    body: 'HMAC 签名的开放 API，可对接发货系统。时间窗口 5 分钟、nonce 去重、权限按密钥隔离。',
  },
]

const steps = [
  { n: '01', t: '出卡', d: '后台或代理端批量生成，卡密不连续、不带业务信息。' },
  { n: '02', t: '客户端登录', d: '卡密 + 设备指纹换 access / refresh，令牌库内只存哈希。' },
  { n: '03', t: '持续校验', d: '心跳与消耗按策略扣减，暂停、吊销、封禁即时生效。' },
]
</script>

<template>
  <div class="landing">
    <header class="nav">
      <a class="brand" href="/">Devour<span>Auth</span></a>
      <nav class="links">
        <a href="#features">能力</a>
        <a href="#flow">接入</a>
        <a href="/query">查卡密</a>
        <a :href="adminURL" class="btn sm">进入后台</a>
      </nav>
    </header>

    <section class="hero">
      <div class="glow a"></div>
      <div class="glow b"></div>
      <div class="hero-inner">
        <p class="eyebrow">自托管的软件授权服务</p>
        <h1>判定留在服务端<br />客户端只能听结果</h1>
        <p class="lede">
          卡密、设备绑定、功能权限、代理分销，一套跑在自己服务器上的授权系统。
          令牌只存哈希，管理动作全部进哈希链，吊销即时生效。
        </p>
        <div class="cta">
          <a class="btn primary" href="/query">查询卡密状态</a>
          <a class="btn ghost" :href="agentURL">代理登录</a>
        </div>
        <ul class="facts">
          <li><strong>6</strong>种卡类型</li>
          <li><strong>403</strong>交叉访问一律拒绝</li>
          <li><strong>15min</strong>access 令牌有效期</li>
          <li><strong>0</strong>客户端可信任的判定</li>
        </ul>
      </div>
    </section>

    <section id="features" class="section">
      <h2>它解决什么</h2>
      <p class="sub">不是把密码学塞进客户端，而是让客户端没有决定权。</p>
      <div class="grid">
        <article v-for="f in features" :key="f.title" class="card">
          <h3>{{ f.title }}</h3>
          <p>{{ f.body }}</p>
        </article>
      </div>
    </section>

    <section id="flow" class="section alt">
      <h2>接入只有三步</h2>
      <div class="steps">
        <div v-for="s in steps" :key="s.n" class="step">
          <span class="num">{{ s.n }}</span>
          <h3>{{ s.t }}</h3>
          <p>{{ s.d }}</p>
        </div>
      </div>
    </section>

    <section class="section">
      <div class="strip">
        <div>
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
      <span>© {{ new Date().getFullYear() }} Devour Auth</span>
      <span class="dot">·</span>
      <a :href="adminURL">管理端</a>
      <a :href="agentURL">代理端</a>
      <a href="/query">卡密查询</a>
    </footer>
  </div>
</template>

<style scoped>
.landing {
  min-height: 100vh;
  background: #f7f8fb;
  color: #12151c;
}

.nav {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 24px;
  backdrop-filter: blur(12px);
  background: rgba(247, 248, 251, 0.82);
  border-bottom: 1px solid rgba(18, 21, 28, 0.06);
}
.brand {
  font-weight: 800;
  font-size: 18px;
  letter-spacing: -0.02em;
  text-decoration: none;
  color: inherit;
}
.brand span {
  color: #4f46e5;
}
.links {
  display: flex;
  align-items: center;
  gap: 20px;
  font-size: 14px;
}
.links a {
  color: #4a5568;
  text-decoration: none;
}
.links a:hover {
  color: #12151c;
}

.hero {
  position: relative;
  overflow: hidden;
  padding: 88px 24px 72px;
  background: radial-gradient(120% 100% at 50% 0%, #1b1f3b 0%, #0b0d18 60%, #07080f 100%);
  color: #f4f6ff;
}
.glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.55;
  pointer-events: none;
}
.glow.a {
  width: 420px;
  height: 420px;
  background: #4f46e5;
  top: -160px;
  left: 8%;
}
.glow.b {
  width: 360px;
  height: 360px;
  background: #06b6d4;
  top: -120px;
  right: 6%;
  opacity: 0.35;
}
.hero-inner {
  position: relative;
  max-width: 760px;
  margin: 0 auto;
  text-align: center;
}
.eyebrow {
  margin: 0 0 14px;
  font-size: 13px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #8b93ff;
}
.hero h1 {
  margin: 0 0 18px;
  font-size: clamp(30px, 5vw, 52px);
  line-height: 1.15;
  letter-spacing: -0.03em;
}
.lede {
  margin: 0 auto 28px;
  max-width: 560px;
  font-size: 16px;
  line-height: 1.75;
  color: #b9c0da;
}
.cta {
  display: flex;
  gap: 12px;
  justify-content: center;
  flex-wrap: wrap;
}
.facts {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 28px;
  margin: 44px 0 0;
  padding: 0;
  list-style: none;
  font-size: 13px;
  color: #9aa3c4;
}
.facts strong {
  display: block;
  font-size: 22px;
  color: #fff;
  font-weight: 700;
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 10px 18px;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
  border: 1px solid transparent;
  transition: transform 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
}
.btn:hover {
  transform: translateY(-1px);
}
.btn.primary {
  background: #4f46e5;
  color: #fff;
  box-shadow: 0 10px 24px -10px rgba(79, 70, 229, 0.8);
}
.btn.primary:hover {
  background: #4338ca;
}
.btn.ghost {
  border-color: rgba(255, 255, 255, 0.22);
  color: #e7eaff;
}
.btn.ghost:hover {
  background: rgba(255, 255, 255, 0.08);
}
.btn.sm {
  padding: 7px 14px;
  font-size: 13px;
  background: #12151c;
  color: #fff;
}

.section {
  max-width: 1080px;
  margin: 0 auto;
  padding: 72px 24px;
}
.section.alt {
  background: #fff;
  max-width: none;
  border-top: 1px solid rgba(18, 21, 28, 0.06);
  border-bottom: 1px solid rgba(18, 21, 28, 0.06);
}
.section.alt > h2,
.section.alt > .steps {
  max-width: 1080px;
  margin-left: auto;
  margin-right: auto;
}
.section h2 {
  margin: 0 0 8px;
  font-size: 26px;
  letter-spacing: -0.02em;
}
.sub {
  margin: 0 0 32px;
  color: #5b6478;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
}
.card {
  padding: 22px;
  border-radius: 14px;
  background: #fff;
  border: 1px solid rgba(18, 21, 28, 0.07);
  box-shadow: 0 1px 2px rgba(18, 21, 28, 0.04);
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.card:hover {
  transform: translateY(-2px);
  box-shadow: 0 16px 32px -20px rgba(18, 21, 28, 0.35);
}
.card h3 {
  margin: 0 0 8px;
  font-size: 16px;
}
.card p {
  margin: 0;
  font-size: 14px;
  line-height: 1.7;
  color: #5b6478;
}

.steps {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 18px;
}
.step .num {
  font-family: ui-monospace, monospace;
  font-size: 13px;
  color: #4f46e5;
}
.step h3 {
  margin: 6px 0 6px;
  font-size: 17px;
}
.step p {
  margin: 0;
  font-size: 14px;
  line-height: 1.7;
  color: #5b6478;
}

.strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
  padding: 32px;
  border-radius: 18px;
  background: linear-gradient(120deg, #eef0ff 0%, #e6fbff 100%);
}
.strip h2 {
  margin: 0 0 6px;
  font-size: 22px;
}
.strip p {
  margin: 0;
  color: #4a5568;
  font-size: 14px;
}
.strip-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  justify-content: center;
  padding: 28px 24px 40px;
  font-size: 13px;
  color: #6b7280;
}
.footer a {
  color: #4a5568;
  text-decoration: none;
}
.footer a:hover {
  color: #12151c;
}
.dot {
  opacity: 0.5;
}

@media (max-width: 640px) {
  .links {
    gap: 12px;
    font-size: 13px;
  }
  .links a:not(.btn) {
    display: none;
  }
  .hero {
    padding: 64px 18px 56px;
  }
  .facts {
    gap: 18px;
  }
  .strip {
    padding: 24px;
  }
}
</style>
