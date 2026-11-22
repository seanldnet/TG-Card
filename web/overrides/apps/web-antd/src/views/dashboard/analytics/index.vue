<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useRouter } from 'vue-router';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface Bot { id: string; name: string; username: string; status: number; }
interface Product { id: string; name: string; status: number; stockCount: number; priceMinor: number; currency: string; }
interface Order { id: string; orderNo: string; productName: string; amountMinor: number; currency: string; status: string; createdAt: string; }
interface Ticket { id: string; status: string; }
interface DashboardMetrics {
  checkedAt: string;
  delivery: { deliveredCount: number; deliveryFailedCount: number; deliveryUncertainWindowCount: number; deliverySuccessRate: number; deliveryUncertainCount: number; deliveryUncertainOldestAt: string; deliveryUncertainOldestAgeSeconds: number; procurementUncertainCount: number; procurementUncertainOldestAt: string; procurementUncertainOldestAgeSeconds: number; refundConflictCount: number; };
  upstreamSync: { enabled?: boolean; status: string; job?: { status: string; totalItems: number; processedItems: number; currentPage: number; errorCount: number; progressPercent: number; lastError: string; }; };
  cap: { success: number; failure: number; total: number; successRatePercent: number; window: string; };
  api: { p95Milliseconds: number; sampleCount: number; window: string; };
  databasePool: { open: number; inUse: number; idle: number; maxOpen: number; waitCount: number; waitDurationMilliseconds: number; };
  alerts: { refundConflicts: number; deliveryUncertain: number; procurementUncertain: number; };
}
const router = useRouter();
const { width } = useWindowSize();
const isMobile = computed(() => width.value < 768);
const loading = ref(false);
const bots = ref<Bot[]>([]);
const products = ref<Product[]>([]);
const orders = ref<Order[]>([]);
const tickets = ref<Ticket[]>([]);
const metrics = ref<DashboardMetrics | null>(null);
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
      requestClient.get<DashboardMetrics>('/dashboard/metrics'),
    ]);
    if (results[0].status === 'fulfilled') bots.value = results[0].value;
    if (results[1].status === 'fulfilled') products.value = results[1].value;
    if (results[2].status === 'fulfilled') orders.value = results[2].value;
    if (results[3].status === 'fulfilled') tickets.value = results[3].value;
    if (results[4].status === 'fulfilled') metrics.value = results[4].value;
    const failures = results.filter((x) => x.status === 'rejected').length;
    if (failures === 5) message.error($t('tg-card.analytics.loadFailed'));
    else if (failures > 0) message.warning($t('tg-card.analytics.partialLoad'));
    loadedAt.value = new Date().toLocaleString();
  } finally {
    loading.value = false;
  }
}
function statusText(status: string) {
  const labels: Record<string,string> = { pending_payment:$t('tg-card.analytics.orderStatus.pendingPayment'), paid:$t('tg-card.analytics.orderStatus.paid'), delivering:$t('tg-card.analytics.orderStatus.delivering'), delivered:$t('tg-card.analytics.orderStatus.delivered'), delivery_failed:$t('tg-card.analytics.orderStatus.deliveryFailed'), expired:$t('tg-card.analytics.orderStatus.expired'), cancelled:$t('tg-card.analytics.orderStatus.cancelled'), failed:$t('tg-card.analytics.orderStatus.failed') };
  return labels[status] || status;
}
function statusColor(status: string) {
  if (status === 'delivered') return 'success';
  if (['paid','delivering'].includes(status)) return 'processing';
  if (['pending_payment'].includes(status)) return 'warning';
  if (['delivery_failed','failed'].includes(status)) return 'error';
  return 'default';
}
function formatDuration(seconds: number) {
  if (!seconds || seconds < 0) return '—';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} ${$t('tg-card.analytics.duration.minutes')}`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} ${$t('tg-card.analytics.duration.hours')}`;
  return `${Math.floor(seconds / 86400)} ${$t('tg-card.analytics.duration.days')}`;
}
function syncStatusText(value: string) {
  const labels: Record<string,string> = { not_started:$t('tg-card.analytics.syncStatus.notStarted'), idle:$t('tg-card.analytics.syncStatus.idle'), queued:$t('tg-card.analytics.syncStatus.queued'), running:$t('tg-card.analytics.syncStatus.running'), cleaning:$t('tg-card.analytics.syncStatus.cleaning'), success:$t('tg-card.analytics.syncStatus.success'), failed:$t('tg-card.analytics.syncStatus.failed'), paused:$t('tg-card.analytics.syncStatus.paused'), cancelled:$t('tg-card.analytics.syncStatus.cancelled') };
  return labels[value] || value || $t('tg-card.analytics.syncStatus.unknown');
}
function open(path: string) { void router.push(path); }
function formatTime(value: string) { const d = new Date(value); return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString(); }
onMounted(loadDashboard);
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
    <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="m-0 text-2xl font-semibold">{{$t('tg-card.analytics.title')}}</h1>
        <p class="mt-1 mb-0 text-sm text-gray-500">{{$t('tg-card.analytics.subtitle')}}</p>
      </div>
      <a-button :loading="loading" @click="loadDashboard">{{$t('tg-card.analytics.refresh')}</a-button>
    </div>

    <a-row v-if="metrics" :gutter="[12,12]">
      <a-col :xs="12" :xl="6"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.deliverySuccessRate')}}</div>
        <div class="mt-1 text-2xl font-semibold">{{ (metrics.delivery.deliverySuccessRate * 100).toFixed(1) }}%</div>
        <a-progress class="mt-2" :percent="Math.round(metrics.delivery.deliverySuccessRate * 100)" :show-info="false"/>
        <div class="mt-1 text-xs text-gray-500">{{$t('tg-card.analytics.success')}} {{ metrics.delivery.deliveredCount }} · {{$t('tg-card.analytics.failed')}} {{ metrics.delivery.deliveryFailedCount }}</div>
      </a-card></a-col>
      <a-col :xs="12" :xl="6"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.uncertainDelivery')}}</div>
        <div class="mt-1 text-2xl font-semibold">{{ metrics.delivery.deliveryUncertainCount }}</div>
        <div class="mt-2 text-xs text-gray-500">{{$t('tg-card.analytics.oldestWait')}} {{ formatDuration(metrics.delivery.deliveryUncertainOldestAgeSeconds) }}</div>
        <div class="mt-1 text-xs text-gray-500">{{$t('tg-card.analytics.newUncertain24h')}} {{ metrics.delivery.deliveryUncertainWindowCount }}</div>
      </a-card></a-col>
      <a-col :xs="12" :xl="6"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.refundConflict')}}</div>
        <div class="mt-1 text-2xl font-semibold" :class="metrics.alerts.refundConflicts ? 'text-red-600' : ''">{{ metrics.alerts.refundConflicts }}</div>
        <div class="mt-2 text-xs text-gray-500">{{$t('tg-card.analytics.procurementUncertain')}} {{ metrics.alerts.procurementUncertain }} ({{$t('tg-card.analytics.oldestWait')}} {{ formatDuration(metrics.delivery.procurementUncertainOldestAgeSeconds) }})</div>
        <div class="mt-1 text-xs text-gray-500">{{$t('tg-card.analytics.deliveryUncertain')}} {{ metrics.alerts.deliveryUncertain }}</div>
      </a-card></a-col>
      <a-col :xs="12" :xl="6"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.upstreamCatalogSync')}}</div>
        <div class="mt-1 text-lg font-semibold">{{ syncStatusText(metrics.upstreamSync.job?.status || metrics.upstreamSync.status) }}</div>
        <a-progress v-if="metrics.upstreamSync.job" class="mt-2" :percent="Math.round(metrics.upstreamSync.job.progressPercent)" :show-info="false"/>
        <div v-if="metrics.upstreamSync.job" class="mt-1 text-xs text-gray-500">{{ metrics.upstreamSync.job.processedItems }} / {{ metrics.upstreamSync.job.totalItems }} {{$t('tg-card.analytics.syncProgressUnit')}} · {{$t('tg-card.analytics.syncPage')}} {{ metrics.upstreamSync.job.currentPage }} 页</div>
        <div v-else class="mt-2 text-xs text-gray-500">{{$t('tg-card.analytics.noActiveSync')}}</div>
      </a-card></a-col>
    </a-row>
    <a-row v-if="metrics" :gutter="[12,12]">
      <a-col :xs="12" :xl="8"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.capSuccessRate')}}</div>
        <div class="mt-1 text-2xl font-semibold">{{ metrics.cap.total ? metrics.cap.successRatePercent.toFixed(1) + '%' : '{{$t('tg-card.analytics.noSample')}}' }}</div>
        <div class="mt-2 text-xs text-gray-500">{{$t('tg-card.analytics.success')}} {{ metrics.cap.success }} · {{$t('tg-card.analytics.failed')}} {{ metrics.cap.failure }}（{{$t('tg-card.analytics.processLifetime')}}）</div>
      </a-card></a-col>
      <a-col :xs="12" :xl="8"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.apiP95')}}</div>
        <div class="mt-1 text-2xl font-semibold">{{ metrics.api.sampleCount ? metrics.api.p95Milliseconds.toFixed(1) + ' ms' : '{{$t('tg-card.analytics.noSample')}}' }}</div>
        <div class="mt-2 text-xs text-gray-500">{{$t('tg-card.analytics.recentApiRequests')}}: {{ metrics.api.sampleCount }}</div>
      </a-card></a-col>
      <a-col :xs="12" :xl="8"><a-card :bordered="false" class="h-full">
        <div class="text-xs text-gray-500">{{$t('tg-card.analytics.databasePool')}}</div>
        <div class="mt-1 text-2xl font-semibold">{{ metrics.databasePool.inUse }} / {{ metrics.databasePool.maxOpen || '∞' }}</div>
        <div class="mt-2 text-xs text-gray-500">{{$t('tg-card.analytics.idle')}} {{ metrics.databasePool.idle }} · {{$t('tg-card.analytics.waitCount')}} {{ metrics.databasePool.waitCount }}</div>
      </a-card></a-col>
    </a-row>
    <a-row :gutter="[12,12]">
      <a-col v-for="metric in [
        { title: $t('tg-card.analytics.bots'), value: bots.length, hint: $t('tg-card.analytics.runningBots', { count: activeBots }), path: '/bots', icon: 'mdi:robot-outline', tone: 'text-blue-600 bg-transparent' },
        { title: '{{$t('tg-card.analytics.product')}}与库存', value: products.length, hint: `上架 ${activeProducts} 件 · 库存 ${stockCount}`, path: '/catalog', icon: 'mdi:package-variant-closed', tone: 'text-blue-600 bg-transparent' },
        { title: $t('tg-card.analytics.orders'), value: orders.length, hint: $t('tg-card.analytics.pendingOrders', { count: pendingOrders }), path: '/orders', icon: 'mdi:receipt-text-outline', tone: 'text-blue-600 bg-transparent' },
        { title: $t('tg-card.analytics.pendingSupport'), value: openTickets, hint: $t('tg-card.analytics.deliveryIssues', { count: deliveryIssues }), path: '/support', icon: 'mdi:headset', tone: 'text-blue-600 bg-transparent' },
      ]" :key="metric.path" :xs="12" :sm="12" :xl="6">
        <a-card :bordered="false" hoverable class="h-full cursor-pointer" :body-style="{ padding: '16px 18px' }" @click="open(metric.path)">
          <div class="flex items-center justify-between gap-2">
            <div class="min-w-0"><div class="text-xs text-gray-500">{{ metric.title }}</div><div class="mt-2 text-2xl font-semibold leading-none">{{ metric.value }}</div><div class="mt-2 truncate text-xs text-gray-500">{{ metric.hint }}</div></div>
            <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon :icon="metric.icon" class="block text-2xl" /></span>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-card class="mt-4" :bordered="false">
      <template #title>{{$t('tg-card.analytics.shortcuts')}}</template>
      <a-row :gutter="[12,12]">
        <a-col v-for="item in [
          { title:$t('tg-card.analytics.botManagement'), desc:$t('tg-card.analytics.botDescription'), path:'/bots' },
          { title:'{{$t('tg-card.analytics.product')}}与卡密', desc:'{{$t('tg-card.analytics.product')}}分类、售价、库存导入', path:'/catalog' },
          { title:$t('tg-card.analytics.orderManagement'), desc:'订单{{$t('tg-card.analytics.status')}}与发货重试', path:'/orders' },
          { title:$t('tg-card.analytics.paymentTransactions'), desc:$t('tg-card.analytics.paymentDescription'), path:'/payments/transactions' },
          { title:$t('tg-card.analytics.supportTickets'), desc:$t('tg-card.analytics.supportDescription'), path:'/support' },
          { title:$t('tg-card.analytics.paymentSettings'), desc:$t('tg-card.analytics.paymentSettingsDescription'), path:'/settings/payment' },
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
          <span>{{$t('tg-card.analytics.recentOrders')}}</span>
          <a-button type="link" @click="open('/orders')">{{$t('tg-card.analytics.allOrders')}}</a-button>
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
        <a-empty v-if="!loading && recentOrders.length === 0" description="{{$t('tg-card.analytics.emptyOrders')}}" />
      </div>
      <a-table v-else :data-source="recentOrders" :loading="loading" row-key="id" :pagination="false" :scroll="{ x: 700 }">
        <a-table-column title="{{$t('tg-card.analytics.orderNumber')}}" data-index="orderNo" key="orderNo" />
        <a-table-column title="{{$t('tg-card.analytics.product')}}" data-index="productName" key="productName" />
        <a-table-column title="{{$t('tg-card.analytics.amount')}}" key="amount">
          <template #default="{ record }">{{ (record.amountMinor / 100).toFixed(2) }} {{ record.currency }}</template>
        </a-table-column>
        <a-table-column title="{{$t('tg-card.analytics.status')}}" key="status">
          <template #default="{ record }"><a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag></template>
        </a-table-column>
        <a-table-column title="{{$t('tg-card.analytics.createdAt')}}" key="createdAt">
          <template #default="{ record }">{{ formatTime(record.createdAt) }}</template>
        </a-table-column>
        <template #emptyText><a-empty description="{{$t('tg-card.analytics.emptyOrders')}}" /></template>
      </a-table>
    </a-card>
    <div class="mt-3 text-right text-xs text-gray-400">{{$t('tg-card.analytics.updatedAt')}}：{{ loadedAt || '{{$t('tg-card.analytics.loading')}}' }}</div>
  </div>
</template>
