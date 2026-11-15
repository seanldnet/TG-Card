<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface OrderOption { id: string; orderNo: string; productName: string; status: string; paymentAmount: string; paymentReference?: string; }
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
const orders = ref<OrderOption[]>([]);
const reconcileOpen = ref(false);
const reconcileLoading = ref(false);
const selectedTransaction = ref<PaymentTransaction | null>(null);
const selectedOrderId = ref('');
const reconciliationNote = ref('');
const filterStatus = ref('all');
const keyword = ref('');
const unmatchedCount = computed(() => transactions.value.filter((x) => x.status === 'unmatched').length);
const reviewCount = computed(() => transactions.value.filter((x) => x.status === 'reconciliation_required').length);
const matchedCount = computed(() => transactions.value.filter((x) => x.status === 'matched').length);
const reconcileCandidates = computed(() => {
  if (!selectedTransaction.value) return [];
  return orders.value.filter((order) => {
    const amount = Math.round(Number(order.paymentAmount || 0) * 1_000_000);
    const stateOK = ['pending_payment', 'expired'].includes(order.status) || (order.paymentReference === selectedTransaction.value?.transactionId && ['paid', 'delivering', 'delivery_failed', 'delivered'].includes(order.status));
    return stateOK && amount === selectedTransaction.value?.amountAtomic;
  });
});
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
async function loadOrders() {
  try { orders.value = await requestClient.get<OrderOption[]>('/orders'); }
  catch { message.error('订单列表加载失败，无法进行人工补单'); }
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
function openReconcile(item: PaymentTransaction) {
  selectedTransaction.value = item;
  selectedOrderId.value = item.orderId || '';
  reconciliationNote.value = item.reconciliationNote || '';
  reconcileOpen.value = true;
  void loadOrders();
}
async function submitReconciliation() {
  if (!selectedTransaction.value || !selectedOrderId.value) { message.warning('请选择与到账金额一致的订单'); return; }
  reconcileLoading.value = true;
  try {
    await requestClient.post('/payment/transactions/' + selectedTransaction.value.transactionId + '/reconcile', { orderId: selectedOrderId.value, note: reconciliationNote.value });
    message.success('交易核对完成，系统已自动尝试发货');
    reconcileOpen.value = false;
    await Promise.all([loadTransactions(), loadOrders()]);
  } catch { message.error('人工核对失败，请检查金额、收款地址、订单状态及库存'); }
  finally { reconcileLoading.value = false; }
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
        <a-table-column title="操作" key="actions" :width="130" fixed="right">
          <template #default="{ record }"><a-button v-if="record.status !== 'matched'" type="link" size="small" @click="openReconcile(record)">人工关联 / 补单</a-button><span v-else class="text-gray-400">已处理</span></template>
        </a-table-column>
        <a-table-column title="首次入库时间" key="firstSeenAt" :width="190">
          <template #default="{ record }">{{ formatTime(record.firstSeenAt) }}</template>
        </a-table-column>
        <template #emptyText><a-empty description="暂无链上交易流水；启用 TRC20 收款并收到转账后，这里会自动记录" /></template>
      </a-table>
      <a-alert class="mt-4" type="info" show-icon message="人工补单会校验当前收款地址、交易金额、订单创建时间及库存；逾期订单只有在仍有可用卡密时才会补发。退款功能尚未开放。" />
    </a-card>
    <a-modal v-model:open="reconcileOpen" title="人工核对链上交易" :confirm-loading="reconcileLoading" ok-text="确认关联并尝试发货" cancel-text="取消" @ok="submitReconciliation">
      <a-alert class="mb-3" type="warning" show-icon message="确认后会将订单标记为已付款，并尝试通过对应 Telegram Bot 发货。请先核对交易哈希、收款地址和到账金额。" />
      <div v-if="selectedTransaction" class="mb-3 text-sm">到账金额：<strong>{{ formatAmount(selectedTransaction) }} USDT</strong><br />交易哈希：{{ selectedTransaction.transactionId }}</div>
      <div class="mb-2 text-sm font-medium">匹配订单（仅显示金额完全一致的订单）</div>
      <a-select v-model:value="selectedOrderId" class="w-full" show-search option-filter-prop="label" placeholder="选择订单">
        <a-select-option v-for="order in reconcileCandidates" :key="order.id" :value="order.id" :label="order.orderNo + ' ' + order.productName">{{ order.orderNo }} · {{ order.productName }} · {{ order.status }}</a-select-option>
      </a-select>
      <div class="mt-3 mb-2 text-sm font-medium">核对备注</div>
      <a-textarea v-model:value="reconciliationNote" :rows="3" :maxlength="1000" show-count placeholder="填写人工核对依据（选填）" />
      <div v-if="reconcileCandidates.length === 0" class="mt-2 text-sm text-orange-600">没有找到金额完全一致且状态允许处理的订单，请勿强行补单。</div>
    </a-modal>
  </div>
</template>
