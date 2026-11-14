<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface Order {
  id: string;
  orderNo: string;
  botId: string;
  telegramUserId: string;
  telegramChatId: string;
  productName: string;
  unitPriceMinor: number;
  quantity: number;
  amountMinor: number;
  paymentAmount: string;
  paymentMethod?: string;
  paymentReference?: string;
  currency: string;
  status: string;
  expiresAt: string;
  createdAt: string;
  paidAt?: string;
  deliveredAt?: string;
  deliveryAttempts: number;
  deliveryError?: string;
  stockReserved: boolean;
}
const loading = ref(false);
const orders = ref<Order[]>([]);
const pendingCount = computed(() => orders.value.filter((item) => item.status === 'pending_payment').length);
const reservedCount = computed(() => orders.value.filter((item) => item.stockReserved).length);
const failedCount = computed(() => orders.value.filter((item) => item.status === 'delivery_failed').length);
const retryingId = ref('');

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    pending_payment: '待付款',
    paid: '已付款',
    delivered: '已发货',
    delivering: '发货中',
    delivery_failed: '发货失败',
    expired: '已过期',
    cancelled: '已取消',
    failed: '异常',
  };
  return labels[status] ?? status;
}
function statusColor(status: string) {
  if (status === 'delivered') return 'success';
  if (status === 'paid' || status === 'delivering') return 'processing';
  if (status === 'delivery_failed') return 'error';
  if (status === 'pending_payment') return 'warning';
  if (status === 'expired' || status === 'cancelled' || status === 'failed') return 'default';
  return 'blue';
}
function paymentMethodLabel(value?: string) {
  if (value === 'trc20-usdt') return 'TRC20 USDT';
  return value || '-';
}
function formatTime(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
async function loadOrders() {
  loading.value = true;
  try {
    orders.value = await requestClient.get<Order[]>('/orders');
  } catch {
    message.error('订单列表加载失败');
  } finally {
    loading.value = false;
  }
}
async function retryDelivery(order: Order) {
  retryingId.value = order.id;
  try {
    await requestClient.post('/orders/' + order.id + '/retry-delivery');
    message.success('订单 ' + order.orderNo + ' 已发货');
    await loadOrders();
  } catch {
    message.error('发货重试失败，请检查机器人状态及错误信息');
    await loadOrders();
  } finally {
    retryingId.value = '';
  }
}
onMounted(loadOrders);
</script>

<template>
  <div class="p-5">
    <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-4">
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">订单总数（最近 500 笔）</div>
        <div class="mt-2 text-2xl font-semibold">{{ orders.length }}</div>
      </a-card>
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">待付款订单</div>
        <div class="mt-2 text-2xl font-semibold">{{ pendingCount }}</div>
      </a-card>
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">当前占用库存</div>
        <div class="mt-2 text-2xl font-semibold">{{ reservedCount }}</div>
      </a-card>
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">发货异常</div>
        <div class="mt-2 text-2xl font-semibold">{{ failedCount }}</div>
      </a-card>
    </div>
    <a-card :bordered="false">
      <template #title>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-lg font-semibold">订单管理</div>
            <div class="mt-1 text-sm text-gray-500">查看 Telegram 用户订单、付款状态、库存预留及自动发货重试情况</div>
          </div>
          <a-button :loading="loading" @click="loadOrders">刷新订单</a-button>
        </div>
      </template>
      <a-table :data-source="orders" :loading="loading" row-key="id" :scroll="{ x: 1100 }">
        <a-table-column title="订单号" data-index="orderNo" key="orderNo" :width="190" />
        <a-table-column title="商品" data-index="productName" key="productName" />
        <a-table-column title="商品金额" key="amount" :width="130">
          <template #default="{ record }">{{ (record.amountMinor / 100).toFixed(2) }} {{ record.currency }}</template>
        </a-table-column>
        <a-table-column title="链上应付金额" key="paymentAmount" :width="150">
          <template #default="{ record }">{{ record.paymentAmount === '0.000000' ? '-' : record.paymentAmount + ' USDT' }}</template>
        </a-table-column>
        <a-table-column title="支付方式" key="paymentMethod" :width="120">
          <template #default="{ record }">{{ paymentMethodLabel(record.paymentMethod) }}</template>
        </a-table-column>
        <a-table-column title="链上交易" key="paymentReference" :width="190">
          <template #default="{ record }">
            <a v-if="record.paymentReference" :href="`https://tronscan.org/#/transaction/${record.paymentReference}`" target="_blank" rel="noreferrer">
              {{ record.paymentReference.slice(0, 12) }}…
            </a>
            <span v-else class="text-gray-400">-</span>
          </template>
        </a-table-column>
        <a-table-column title="Telegram 用户" data-index="telegramUserId" key="telegramUserId" :width="150" />
        <a-table-column title="订单状态" key="status" :width="110">
          <template #default="{ record }"><a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag></template>
        </a-table-column>
        <a-table-column title="库存预留" key="stockReserved" :width="100">
          <template #default="{ record }"><a-tag :color="record.stockReserved ? 'processing' : 'default'">{{ record.stockReserved ? '已预留' : '未预留' }}</a-tag></template>
        </a-table-column>
        <a-table-column title="发货重试" key="delivery" :width="120">
          <template #default="{ record }">
            <a-button
              v-if="record.status === 'paid' || record.status === 'delivery_failed'"
              type="link"
              size="small"
              :loading="retryingId === record.id"
              @click="retryDelivery(record)"
            >重试发货</a-button>
            <span v-else class="text-gray-400">-</span>
          </template>
        </a-table-column>
        <a-table-column title="发货尝试次数" data-index="deliveryAttempts" key="deliveryAttempts" :width="120">
          <template #default="{ record }">{{ record.deliveryAttempts }}</template>
        </a-table-column>
        <a-table-column title="最近发货错误" data-index="deliveryError" key="deliveryError" :width="220" ellipsis />
        <a-table-column title="创建时间" key="createdAt" :width="190">
          <template #default="{ record }">{{ formatTime(record.createdAt) }}</template>
        </a-table-column>
        <a-table-column title="付款截止" key="expiresAt" :width="190">
          <template #default="{ record }">{{ formatTime(record.expiresAt) }}</template>
        </a-table-column>
      </a-table>
      <a-alert class="mt-3" type="info" show-icon message="TRC20 USDT 通过 TronGrid 已确认交易自动核验；请按 Bot 提供的六位小数金额转账。付款后进入带租约的自动发货队列，失败时可在此重试。" />
    </a-card>
  </div>
</template>
