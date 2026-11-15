<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface PaymentTransaction {
  id: string;
  provider: string;
  transactionId: string;
  fromAddress: string;
  toAddress: string;
  amountAtomic: number;
  tokenDecimals: number;
  blockTimestampMs: number;
  status: 'matched' | 'unmatched' | 'reconciliation_required' | string;
  orderId?: string;
  reconciliationNote?: string;
  firstSeenAt: string;
  updatedAt: string;
}
const loading = ref(false);
const transactions = ref<PaymentTransaction[]>([]);
const filterStatus = ref('all');
const keyword = ref('');
const unmatchedCount = computed(() => transactions.value.filter((x) => x.status === 'unmatched').length);
const reviewCount = computed(() => transactions.value.filter((x) => x.status === 'reconciliation_required').length);
const matchedCount = computed(() => transactions.value.filter((x) => x.status === 'matched').length);
const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  return transactions.value.filter((item) => {
    const statusOK = filterStatus.value === 'all' || item.status === filterStatus.value;
    const text = [item.transactionId, item.fromAddress, item.toAddress, item.orderId || '', item.reconciliationNote || ''].join(' ').toLowerCase();
    return statusOK && (!q || text.includes(q));
  });
});
function statusLabel(value: string) {
  const labels: Record<string, string> = { matched: '已匹配订单', unmatched: '未匹配', reconciliation_required: '需要人工核对' };
  return labels[value] || value;
}
function statusColor(value: string) {
  if (value === 'matched') return 'success';
  if (value === 'reconciliation_required') return 'error';
  if (value === 'unmatched') return 'warning';
  return 'default';
}
function formatAmount(item: PaymentTransaction) {
  const scale = 10 ** item.tokenDecimals;
  return (item.amountAtomic / scale).toFixed(item.tokenDecimals);
}
function formatTime(value: number | string) {
  const date = typeof value === 'number' ? new Date(value) : new Date(value);
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString();
}
function shortAddress(value: string) {
  return value ? value.slice(0, 8) + '…' + value.slice(-6) : '-';
}
async function loadTransactions() {
  loading.value = true;
  try {
    transactions.value = await requestClient.get<PaymentTransaction[]>('/payment/transactions');
  } catch {
    message.error('支付流水加载失败');
  } finally {
    loading.value = false;
  }
}
function copy(value: string) {
  if (!value) return;
  navigator.clipboard?.writeText(value).then(() => message.success('已复制')).catch(() => message.error('复制失败'));
}
onMounted(loadTransactions);
</script>

<template>
  <div class="p-5">
    <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-4">
      <a-card :bordered="false"><div class="text-sm text-gray-500">链上流水（最近 500 笔）</div><div class="mt-2 text-2xl font-semibold">{{ transactions.length }}</div></a-card>
      <a-card :bordered="false"><div class="text-sm text-gray-500">已匹配订单</div><div class="mt-2 text-2xl font-semibold text-green-600">{{ matchedCount }}</div></a-card>
      <a-card :bordered="false"><div class="text-sm text-gray-500">未匹配交易</div><div class="mt-2 text-2xl font-semibold text-orange-500">{{ unmatchedCount }}</div></a-card>
      <a-card :bordered="false"><div class="text-sm text-gray-500">需要人工核对</div><div class="mt-2 text-2xl font-semibold text-red-500">{{ reviewCount }}</div></a-card>
    </div>
    <a-card :bordered="false">
      <template #title>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-lg font-semibold">支付交易流水</div>
            <div class="mt-1 text-sm text-gray-500">记录 TronGrid 检测到的已确认 TRC20 USDT 转账；交易哈希去重，异常记录不会因轮询而丢失</div>
          </div>
          <a-button :loading="loading" @click="loadTransactions">刷新流水</a-button>
        </div>
      </template>
      <div class="mb-4 flex flex-wrap gap-3">
        <a-select v-model:value="filterStatus" class="w-52" aria-label="流水状态筛选">
          <a-select-option value="all">全部状态</a-select-option>
          <a-select-option value="matched">已匹配订单</a-select-option>
          <a-select-option value="unmatched">未匹配</a-select-option>
          <a-select-option value="reconciliation_required">需要人工核对</a-select-option>
        </a-select>
        <a-input v-model:value="keyword" allow-clear class="max-w-md" placeholder="搜索交易哈希、地址、订单 ID 或核对备注" />
      </div>
      <a-table :data-source="filtered" :loading="loading" row-key="id" :pagination="{ pageSize: 15, showSizeChanger: true }" :scroll="{ x: 1250 }">
        <a-table-column title="链上交易哈希" key="transactionId" :width="240" fixed="left">
          <template #default="{ record }">
            <div class="flex items-center gap-2">
              <a :href="`https://tronscan.org/#/transaction/${record.transactionId}`" target="_blank" rel="noreferrer">{{ record.transactionId.slice(0, 12) }}…</a>
              <a-button type="text" size="small" @click="copy(record.transactionId)">复制</a-button>
            </div>
          </template>
        </a-table-column>
        <a-table-column title="到账金额" key="amount" :width="150">
          <template #default="{ record }"><span class="font-semibold">{{ formatAmount(record) }} USDT</span></template>
        </a-table-column>
        <a-table-column title="付款地址" key="fromAddress" :width="170">
          <template #default="{ record }"><a-tooltip :title="record.fromAddress">{{ shortAddress(record.fromAddress) }}</a-tooltip></template>
        </a-table-column>
        <a-table-column title="收款地址" key="toAddress" :width="170">
          <template #default="{ record }"><a-tooltip :title="record.toAddress">{{ shortAddress(record.toAddress) }}</a-tooltip></template>
        </a-table-column>
        <a-table-column title="链上确认时间" key="blockTimestampMs" :width="190">
          <template #default="{ record }">{{ formatTime(record.blockTimestampMs) }}</template>
        </a-table-column>
        <a-table-column title="处理状态" key="status" :width="140">
          <template #default="{ record }"><a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag></template>
        </a-table-column>
        <a-table-column title="关联订单 ID" data-index="orderId" key="orderId" :width="190">
          <template #default="{ record }">{{ record.orderId || '-' }}</template>
        </a-table-column>
        <a-table-column title="核对备注" data-index="reconciliationNote" key="reconciliationNote" :width="260" ellipsis />
        <a-table-column title="首次入库时间" key="firstSeenAt" :width="190">
          <template #default="{ record }">{{ formatTime(record.firstSeenAt) }}</template>
        </a-table-column>
        <template #emptyText><a-empty description="暂无链上交易流水；启用 TRC20 收款并收到转账后，这里会自动记录" /></template>
      </a-table>
      <a-alert class="mt-4" type="info" show-icon message="当前页面用于追踪和筛选链上入账。人工关联订单、逾期交易补单和退款尚未开放，避免在库存已释放或订单状态不一致时误发卡密。" />
    </a-card>
  </div>
</template>
