<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useRouter } from 'vue-router';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';

interface Bot { id: string; name: string; username: string; status: number; }
interface Product { id: string; name: string; status: number; stockCount: number; priceMinor: number; currency: string; }
interface Order { id: string; orderNo: string; productName: string; amountMinor: number; currency: string; status: string; createdAt: string; }
interface Ticket { id: string; status: string; }
const router = useRouter();
const { width } = useWindowSize();
const isMobile = computed(() => width.value < 768);
const loading = ref(false);
const bots = ref<Bot[]>([]);
const products = ref<Product[]>([]);
const orders = ref<Order[]>([]);
const tickets = ref<Ticket[]>([]);
const loadedAt = ref('');

const activeBots = computed(() => bots.value.filter((x) => x.status === 1).length);
const activeProducts = computed(() => products.value.filter((x) => x.status === 1).length);
const stockCount = computed(() => products.value.reduce((sum, x) => sum + (x.stockCount || 0), 0));
const pendingOrders = computed(() => orders.value.filter((x) => x.status === 'pending_payment').length);
const deliveryIssues = computed(() => orders.value.filter((x) => ['paid', 'delivery_failed'].includes(x.status)).length);
const openTickets = computed(() => tickets.value.filter((x) => !['resolved', 'rejected', 'closed'].includes(x.status)).length);
const recentOrders = computed(() => [...orders.value].sort((a,b) => new Date(b.createdAt).getTime()-new Date(a.createdAt).getTime()).slice(0,8));

async function loadDashboard() {
  loading.value = true;
  try {
    const results = await Promise.allSettled([
      requestClient.get<Bot[]>('/bots'),
      requestClient.get<Product[]>('/catalog/products'),
      requestClient.get<Order[]>('/orders'),
      requestClient.get<Ticket[]>('/support/tickets'),
    ]);
    if (results[0].status === 'fulfilled') bots.value = results[0].value;
    if (results[1].status === 'fulfilled') products.value = results[1].value;
    if (results[2].status === 'fulfilled') orders.value = results[2].value;
    if (results[3].status === 'fulfilled') tickets.value = results[3].value;
    const failures = results.filter((x) => x.status === 'rejected').length;
    if (failures === 4) message.error('经营数据加载失败，请检查登录状态和服务日志');
    else if (failures > 0) message.warning('部分经营数据暂时无法加载，已显示可获取的数据');
    loadedAt.value = new Date().toLocaleString();
  } finally {
    loading.value = false;
  }
}
function statusText(status: string) {
  const labels: Record<string,string> = { pending_payment:'待付款', paid:'已付款', delivering:'发货中', delivered:'已发货', delivery_failed:'发货失败', expired:'已过期', cancelled:'已取消', failed:'异常' };
  return labels[status] || status;
}
function statusColor(status: string) {
  if (status === 'delivered') return 'success';
  if (['paid','delivering'].includes(status)) return 'processing';
  if (['pending_payment'].includes(status)) return 'warning';
  if (['delivery_failed','failed'].includes(status)) return 'error';
  return 'default';
}
function open(path: string) { void router.push(path); }
function formatTime(value: string) { const d = new Date(value); return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(); }
onMounted(loadDashboard);
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
    <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="m-0 text-2xl font-semibold">TG-Card 经营概览</h1>
        <p class="mt-1 mb-0 text-sm text-gray-500">单商户 Telegram 多 Bot 自动发卡商城 · 实时读取业务数据</p>
      </div>
      <a-button :loading="loading" @click="loadDashboard">刷新数据</a-button>
    </div>

    <a-row :gutter="[12,12]">
      <a-col v-for="metric in [
        { title: '机器人', value: bots.length, hint: `运行中 ${activeBots} 个`, path: '/bots', icon: 'mdi:robot-outline', tone: 'text-blue-600 bg-transparent' },
        { title: '商品与库存', value: products.length, hint: `上架 ${activeProducts} 件 · 库存 ${stockCount}`, path: '/catalog', icon: 'mdi:package-variant-closed', tone: 'text-blue-600 bg-transparent' },
        { title: '订单', value: orders.length, hint: `待付款 ${pendingOrders} 笔`, path: '/orders', icon: 'mdi:receipt-text-outline', tone: 'text-blue-600 bg-transparent' },
        { title: '待处理售后', value: openTickets, hint: `发货异常 ${deliveryIssues} 笔`, path: '/support', icon: 'mdi:headset', tone: 'text-blue-600 bg-transparent' },
      ]" :key="metric.path" :xs="12" :sm="12" :xl="6">
        <a-card :bordered="false" hoverable class="h-full cursor-pointer" :body-style="{ padding: '16px 18px' }" @click="open(metric.path)">
          <div class="flex items-center justify-between gap-2">
            <div class="min-w-0"><div class="text-xs text-gray-500">{{ metric.title }}</div><div class="mt-2 text-2xl font-semibold leading-none">{{ metric.value }}</div><div class="mt-2 truncate text-xs text-gray-500">{{ metric.hint }}</div></div>
            <a-avatar :size="44" shape="square" :class="['flex shrink-0 items-center justify-center !bg-transparent !text-blue-600', metric.tone]"><IconifyIcon :icon="metric.icon" class="block text-2xl" /></a-avatar>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-card class="mt-4" :bordered="false">
      <template #title>业务快捷入口</template>
      <a-row :gutter="[12,12]">
        <a-col v-for="item in [
          { title:'机器人管理', desc:'连接 Bot、配置欢迎语', path:'/bots' },
          { title:'商品与卡密', desc:'商品分类、售价、库存导入', path:'/catalog' },
          { title:'订单管理', desc:'订单状态与发货重试', path:'/orders' },
          { title:'支付流水', desc:'链上交易核验与人工补单', path:'/payments/transactions' },
          { title:'售后工单', desc:'处理用户售后请求', path:'/support' },
          { title:'支付设置', desc:'配置 TRON 收款参数', path:'/settings/payment' },
        ]" :key="item.path" :xs="24" :sm="12" :lg="8">
          <a-card size="small" hoverable @click="open(item.path)">
            <div class="font-medium">{{ item.title }}</div>
            <div class="mt-1 text-xs text-gray-500">{{ item.desc }}</div>
          </a-card>
        </a-col>
      </a-row>
    </a-card>

    <a-card class="mt-4" :bordered="false">
      <template #title>
        <div class="flex items-center justify-between gap-3">
          <span>最近订单</span>
          <a-button type="link" @click="open('/orders')">全部订单</a-button>
        </div>
      </template>
      <div v-if="isMobile" class="tg-mobile-list">
        <a-card v-for="record in recentOrders" :key="record.id" size="small">
          <div class="flex min-w-0 items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="break-all font-semibold">{{ record.orderNo }}</div>
              <div class="mt-1 break-words text-sm text-gray-600">{{ record.productName }}</div>
            </div>
            <a-tag class="shrink-0" :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag>
          </div>
          <div class="mt-3 flex items-center justify-between gap-3">
            <div class="text-lg font-bold">{{ (record.amountMinor / 100).toFixed(2) }} {{ record.currency }}</div>
            <div class="text-xs text-gray-500">{{ formatTime(record.createdAt) }}</div>
          </div>
        </a-card>
        <a-empty v-if="!loading && recentOrders.length === 0" description="暂无订单数据" />
      </div>
      <a-table v-else :data-source="recentOrders" :loading="loading" row-key="id" :pagination="false" :scroll="{ x: 700 }">
        <a-table-column title="订单号" data-index="orderNo" key="orderNo" />
        <a-table-column title="商品" data-index="productName" key="productName" />
        <a-table-column title="金额" key="amount">
          <template #default="{ record }">{{ (record.amountMinor / 100).toFixed(2) }} {{ record.currency }}</template>
        </a-table-column>
        <a-table-column title="状态" key="status">
          <template #default="{ record }"><a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag></template>
        </a-table-column>
        <a-table-column title="创建时间" key="createdAt">
          <template #default="{ record }">{{ formatTime(record.createdAt) }}</template>
        </a-table-column>
        <template #emptyText><a-empty description="暂无订单数据" /></template>
      </a-table>
    </a-card>
    <div class="mt-3 text-right text-xs text-gray-400">数据更新时间：{{ loadedAt || '加载中' }}</div>
  </div>
</template>
