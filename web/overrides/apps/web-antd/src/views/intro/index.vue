<script lang="ts" setup>
import { useRouter } from 'vue-router';

defineOptions({ name: 'ProductIntro' });

const router = useRouter();

const features = [
  {
    title: '多 Bot 统一管理',
    description: '一个网页后台接入多个 Telegram Bot。机器人独立配置欢迎语、菜单与客服入口，商品、库存、订单和用户数据统一管理。',
    icon: '🤖',
  },
  {
    title: '商品与卡密库存',
    description: '可视化管理分类、商品详情、售价、上下架和库存。支持批量导入卡密、库存状态追踪与自动分配。',
    icon: '🎟️',
  },
  {
    title: '自动下单与发货',
    description: '用户在 Telegram 内浏览商品、确认订单并完成支付。支付确认后自动锁定库存、交付卡密并保留订单记录。',
    icon: '⚡',
  },
  {
    title: '多渠道支付',
    description: '首期内置 USDT-TRC20 链上收款，并规划接入 GMpay、EPUSDT。支付回调验签、到账确认和重复通知幂等处理。',
    icon: '💳',
  },
  {
    title: '订单售后工单',
    description: '用户可从机器人提交售后、补充凭证和查询进度。管理员可受理工单、补发商品或按规则处理退款。',
    icon: '🛟',
  },
  {
    title: '签到与营销工具',
    description: '支持签到奖励、连续签到、余额积分、优惠券、兑换码和促销活动，帮助商城持续运营。',
    icon: '🎁',
  },
];

const steps = [
  { number: '01', title: '连接机器人', description: '在网页后台安全添加 Bot Token，完成连接测试。' },
  { number: '02', title: '配置商城', description: '设置欢迎介绍、菜单、商品分类、商品价格和卡密库存。' },
  { number: '03', title: '配置收款', description: '启用 USDT-TRC20 或已接入的第三方支付渠道。' },
  { number: '04', title: '自动运营', description: '用户通过 Telegram 自助购买，系统自动确认支付并交付商品。' },
];

function goLogin() {
  void router.push('/auth/login');
}

function scrollToFeatures() {
  document.querySelector('#features')?.scrollIntoView({ behavior: 'smooth' });
}
</script>

<template>
  <main class="intro-page">
    <header class="intro-header">
      <a class="brand" href="/intro" aria-label="TG-Card 首页">
        <span class="brand-mark">TG</span>
        <span>TG-Card</span>
      </a>
      <nav class="header-nav">
        <a href="#features">核心功能</a>
        <a href="#workflow">使用流程</a>
        <a href="#security">安全与稳定</a>
      </nav>
      <a-button type="primary" @click="goLogin">进入管理后台</a-button>
    </header>

    <section class="hero">
      <div class="hero-copy">
        <a-tag color="processing" class="hero-tag">Telegram 自动发卡商城系统</a-tag>
        <h1>让 Telegram Bot<br /><span>成为你的自动发卡商城</span></h1>
        <p class="hero-description">
          TG-Card 是面向单商户的多机器人自动发卡平台。通过一个网页后台管理多个 Telegram Bot，
          统一运营商品、卡密库存、订单、支付与售后，让用户在 Telegram 内完成自助购买。
        </p>
        <div class="hero-actions">
          <a-button type="primary" size="large" @click="goLogin">进入管理后台</a-button>
          <a-button size="large" @click="scrollToFeatures">了解系统功能</a-button>
        </div>
        <div class="hero-points">
          <span>✓ 多 Bot 统一管理</span>
          <span>✓ 支付后自动发货</span>
          <span>✓ TRC20 USDT 收款</span>
        </div>
      </div>
      <div class="hero-visual" aria-label="TG-Card 功能概览">
        <div class="visual-topline"><span class="online-dot"></span> TG-CARD COMMERCE</div>
        <div class="bot-window">
          <div class="bot-avatar">🤖</div>
          <div class="bot-title">自动发卡商城</div>
          <div class="bot-subtitle">欢迎回来，选择你需要的服务</div>
          <div class="bot-product">
            <div><b>高级会员卡</b><small>自动发货 · 库存充足</small></div>
            <strong>12.00 USDT</strong>
          </div>
          <div class="bot-product">
            <div><b>数字兑换码</b><small>付款后即时交付</small></div>
            <strong>5.00 USDT</strong>
          </div>
          <div class="bot-button">🛍️ 浏览全部商品</div>
          <div class="bot-menu"><span>我的订单</span><span>我的余额</span><span>联系客服</span></div>
        </div>
        <div class="float-card float-card-one"><span>⚡</span><div><b>自动交付</b><small>支付确认后发货</small></div></div>
        <div class="float-card float-card-two"><span>🔗</span><div><b>USDT TRC20</b><small>链上收款支持</small></div></div>
      </div>
    </section>

    <section class="trust-strip">
      <div><strong>单商户架构</strong><span>数据统一管理</span></div>
      <div><strong>多个 Telegram Bot</strong><span>独立入口，共用业务核心</span></div>
      <div><strong>网页可视化后台</strong><span>不依赖手动配置 BotFather 菜单</span></div>
      <div><strong>自动化交易流程</strong><span>订单、支付、发货可追踪</span></div>
    </section>

    <section id="features" class="section features-section">
      <div class="section-heading">
        <a-tag color="blue">CORE FEATURES</a-tag>
        <h2>发卡商城运营所需的核心能力</h2>
        <p>从机器人接入到商品交付、售后与营销，集中在同一套管理后台。</p>
      </div>
      <a-row :gutter="[20, 20]">
        <a-col v-for="feature in features" :key="feature.title" :xs="24" :sm="12" :lg="8">
          <a-card class="feature-card" :bordered="false" hoverable>
            <div class="feature-icon">{{ feature.icon }}</div>
            <h3>{{ feature.title }}</h3>
            <p>{{ feature.description }}</p>
          </a-card>
        </a-col>
      </a-row>
    </section>

    <section id="workflow" class="section workflow-section">
      <div class="section-heading">
        <a-tag color="blue">HOW IT WORKS</a-tag>
        <h2>四步启动你的自动发卡业务</h2>
        <p>日常商品、菜单和运营内容都在网页后台完成配置。</p>
      </div>
      <div class="workflow-grid">
        <article v-for="step in steps" :key="step.number" class="workflow-item">
          <span class="step-number">{{ step.number }}</span>
          <h3>{{ step.title }}</h3>
          <p>{{ step.description }}</p>
        </article>
      </div>
    </section>

    <section id="security" class="security-section">
      <div class="security-copy">
        <a-tag color="success">BUILT FOR OPERATIONS</a-tag>
        <h2>关键业务流程，清晰可控</h2>
        <p>平台围绕订单、库存和资金安全设计。支付结果以服务端确认或链上确认结果为准，避免仅凭前端页面或用户截图确认到账。</p>
      </div>
      <div class="security-list">
        <div><span>✓</span><div><b>支付状态可核验</b><small>回调验签、链上交易确认与交易哈希去重</small></div></div>
        <div><span>✓</span><div><b>库存交付有保护</b><small>订单事务、库存锁定与重复发货保护</small></div></div>
        <div><span>✓</span><div><b>运营操作可追踪</b><small>订单记录、售后处理和管理员审计日志</small></div></div>
        <div><span>✓</span><div><b>多 Bot 配置集中管理</b><small>全局默认配置与单 Bot 独立覆盖</small></div></div>
      </div>
    </section>

    <section class="closing-cta">
      <h2>从一个后台，管理你的 Telegram 商城</h2>
      <p>连接机器人，配置商品与库存，让用户通过 Telegram 自助完成购买。</p>
      <a-button type="primary" size="large" @click="goLogin">进入管理后台</a-button>
    </section>

    <footer class="intro-footer">
      <span>© {{ new Date().getFullYear() }} TG-Card</span>
      <span>Telegram 多 Bot 自动发卡商城</span>
    </footer>
  </main>
</template>

<style scoped>
.intro-page{min-height:100vh;background:#f7f9fc;color:#172033;font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.intro-header{height:76px;max-width:1200px;margin:auto;padding:0 28px;display:flex;align-items:center;justify-content:space-between}
.brand{display:flex;align-items:center;gap:10px;color:#15233b;font-size:20px;font-weight:800;text-decoration:none}
.brand-mark{display:grid;place-items:center;width:36px;height:36px;border-radius:11px;background:#1677ff;color:white;font-size:13px}
.header-nav{display:flex;gap:32px}.header-nav a{color:#64748b;text-decoration:none;font-size:14px}.header-nav a:hover{color:#1677ff}
.hero{max-width:1200px;margin:0 auto;padding:76px 28px 88px;display:grid;grid-template-columns:1.08fr .92fr;align-items:center;gap:50px}
.hero-tag{margin-bottom:18px}.hero h1{font-size:clamp(38px,5vw,58px);line-height:1.16;letter-spacing:-2px;font-weight:800;margin:0 0 22px;color:#111d33}.hero h1 span{color:#1677ff}
.hero-description{max-width:590px;color:#63718a;font-size:16px;line-height:1.9}.hero-actions{display:flex;gap:12px;margin-top:30px}.hero-points{display:flex;flex-wrap:wrap;gap:18px;margin-top:26px;color:#53647d;font-size:13px}
.hero-visual{position:relative;min-height:430px;border-radius:28px;background:linear-gradient(140deg,#e8f2ff,#dceaff 55%,#f1f6ff);display:grid;place-items:center;overflow:hidden}
.visual-topline{position:absolute;top:25px;left:28px;color:#5b7397;font-size:10px;letter-spacing:2px;font-weight:800}.online-dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:#20b26b;margin-right:8px}
.bot-window{width:min(330px,76%);padding:24px;border-radius:20px;background:white;box-shadow:0 24px 70px #244d8524;text-align:center}
.bot-avatar{width:54px;height:54px;border-radius:17px;background:#eaf3ff;display:grid;place-items:center;font-size:28px;margin:0 auto 12px}.bot-title{font-size:18px;font-weight:800}.bot-subtitle{font-size:12px;color:#8492a8;margin:5px 0 20px}
.bot-product{display:flex;align-items:center;justify-content:space-between;text-align:left;padding:12px 0;border-top:1px solid #edf1f6}.bot-product b{display:block;font-size:12px}.bot-product small{display:block;color:#94a0b2;font-size:10px;margin-top:4px}.bot-product strong{font-size:11px;color:#1677ff}
.bot-button{background:#1677ff;color:white;border-radius:9px;padding:11px;font-size:12px;font-weight:700;margin-top:12px}.bot-menu{display:flex;gap:5px;margin-top:9px}.bot-menu span{flex:1;background:#f2f6fb;padding:8px 2px;border-radius:7px;font-size:9px;color:#52647e}
.float-card{position:absolute;display:flex;align-items:center;gap:10px;background:white;padding:12px 15px;border-radius:13px;box-shadow:0 12px 32px #244d8520}.float-card>span{font-size:22px}.float-card b,.float-card small{display:block}.float-card b{font-size:11px}.float-card small{font-size:9px;color:#8a97aa;margin-top:3px}.float-card-one{left:12px;bottom:58px}.float-card-two{right:10px;top:95px}
.trust-strip{max-width:1144px;margin:0 auto;padding:24px 30px;background:white;border:1px solid #edf1f6;border-radius:16px;display:grid;grid-template-columns:repeat(4,1fr);gap:18px}
.trust-strip div{display:flex;flex-direction:column;gap:6px}.trust-strip strong{font-size:13px}.trust-strip span{font-size:11px;color:#8996a9}
.section{max-width:1200px;margin:0 auto;padding:100px 28px}.section-heading{text-align:center;margin-bottom:42px}.section-heading h2,.security-copy h2{font-size:32px;letter-spacing:-1px;margin:16px 0 10px;color:#14213a}.section-heading p,.security-copy p{color:#77849a;font-size:14px;line-height:1.8}
.feature-card{height:100%;border-radius:15px;box-shadow:0 6px 24px #1c35550a}.feature-icon{font-size:28px;margin-bottom:18px}.feature-card h3{font-size:17px;font-weight:750;margin-bottom:10px}.feature-card p{font-size:13px;color:#758198;line-height:1.85;margin:0}
.workflow-section{padding-top:35px}.workflow-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:18px}.workflow-item{padding:26px 22px;border-radius:15px;background:white;border:1px solid #edf1f6}.step-number{font-size:13px;color:#1677ff;font-weight:800;letter-spacing:1px}.workflow-item h3{font-size:16px;margin:16px 0 8px}.workflow-item p{font-size:12px;color:#78869b;line-height:1.8;margin:0}
.security-section{max-width:1144px;margin:20px auto 100px;padding:55px 60px;border-radius:22px;background:#111f38;color:white;display:grid;grid-template-columns:1fr 1fr;gap:50px}.security-copy h2{color:white}.security-copy p{color:#b1bfd3}.security-list{display:grid;gap:18px}.security-list>div{display:flex;gap:13px;align-items:flex-start}.security-list>div>span{display:grid;place-items:center;width:23px;height:23px;flex:0 0 23px;border-radius:50%;background:#183f68;color:#62d6a0}.security-list b,.security-list small{display:block}.security-list b{font-size:13px}.security-list small{font-size:11px;color:#9baac0;margin-top:5px}
.closing-cta{text-align:center;padding:65px 24px;background:#eaf3ff}.closing-cta h2{font-size:29px;color:#14213a;margin:0 0 12px}.closing-cta p{font-size:13px;color:#75849b;margin-bottom:25px}
.intro-footer{max-width:1200px;margin:auto;padding:25px 28px;display:flex;justify-content:space-between;color:#93a0b2;font-size:11px}
@media(max-width:800px){.header-nav{display:none}.intro-header{padding:0 18px}.hero{grid-template-columns:1fr;padding:54px 20px 55px;gap:35px}.hero h1{font-size:42px}.hero-visual{min-height:390px}.trust-strip{margin:0 16px;grid-template-columns:repeat(2,1fr);padding:20px}.section{padding:70px 18px}.section-heading h2,.security-copy h2{font-size:26px}.workflow-grid{grid-template-columns:repeat(2,1fr)}.security-section{margin:0 16px 70px;padding:35px 24px;grid-template-columns:1fr;gap:30px}.intro-footer{padding:22px 18px}}
@media(max-width:480px){.hero h1{font-size:36px}.hero-description{font-size:14px}.hero-actions{flex-direction:column}.hero-actions :deep(.ant-btn){width:100%}.hero-points{gap:10px}.workflow-grid{grid-template-columns:1fr}.float-card-one{left:6px}.float-card-two{right:6px}.trust-strip strong{font-size:12px}.trust-strip span{font-size:10px}.closing-cta h2{font-size:24px}.intro-footer{flex-direction:column;gap:8px}}
</style>
