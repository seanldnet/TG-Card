<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { useWindowSize } from '@vueuse/core';
import { requestClient } from '#/api/request';

interface Order {
  id: string; orderNo: string; botId: string; telegramUserId: string; telegramChatId: string;
  productName: string; unitPriceMinor: number; quantity: number; amountMinor: number;
  paymentAmount: string; paymentMethod?: string; paymentReference?: string; currency: string;
  status: string; expiresAt: string; createdAt: string; paidAt?: string; deliveredAt?: string;
  deliveryAttempts: number; deliveryError?: string; stockReserved: boolean;
}
const loading=ref(false), orders=ref<Order[]>([]), retryingId=ref('');
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const pendingCount=computed(()=>orders.value.filter(x=>x.status==='pending_payment').length);
const reservedCount=computed(()=>orders.value.filter(x=>x.stockReserved).length);
const failedCount=computed(()=>orders.value.filter(x=>x.status==='delivery_failed').length);
function statusLabel(status:string){const labels:Record<string,string>={pending_payment:'待付款',paid:'已付款',delivered:'已发货',delivering:'发货中',delivery_failed:'发货失败',expired:'已过期',cancelled:'已取消',failed:'异常'};return labels[status]??status}
function statusColor(status:string){if(status==='delivered')return 'success';if(status==='paid'||status==='delivering')return 'processing';if(status==='delivery_failed')return 'error';if(status==='pending_payment')return 'warning';return 'default'}
function formatTime(value:string){if(!value)return '-';const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString()}
function copy(value:string){if(!value)return;navigator.clipboard?.writeText(value).then(()=>message.success('已复制')).catch(()=>message.error('复制失败'))}
async function loadOrders(){loading.value=true;try{orders.value=await requestClient.get<Order[]>('/orders')}catch{message.error('订单列表加载失败')}finally{loading.value=false}}
async function retryDelivery(order:Order){retryingId.value=order.id;try{await requestClient.post('/orders/'+order.id+'/retry-delivery');message.success('订单 '+order.orderNo+' 已触发发货重试');await loadOrders()}catch{message.error('发货重试失败，请检查机器人状态及错误信息');await loadOrders()}finally{retryingId.value=''}}
onMounted(loadOrders);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-4 md:gap-4">
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">订单总数</div><div class="mt-2 text-2xl font-bold">{{orders.length}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:receipt-text-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">待付款</div><div class="mt-2 text-2xl font-bold">{{pendingCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:clock-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">库存预留</div><div class="mt-2 text-2xl font-bold">{{reservedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:package-variant-closed" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">发货异常</div><div class="mt-2 text-2xl font-bold">{{failedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:alert-circle-outline" class="block text-2xl" /></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3 px-1 py-1"><div class="min-w-0"><div class="text-base font-semibold leading-6">订单管理</div><div class="mt-1 text-xs leading-5 text-gray-500">查看付款状态、库存预留及自动发货重试</div></div><a-button size="small" class="mr-1" :loading="loading" @click="loadOrders">刷新订单</a-button></div></template>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in orders" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
       <div class="flex min-w-0 items-center gap-2"><span class="truncate font-semibold">{{item.orderNo}}</span><a-button size="small" type="text" @click="copy(item.orderNo)">复制</a-button></div>
       <div class="mt-1 break-words text-sm text-gray-600">{{item.productName}}</div>
      </div>
      <a-tag class="shrink-0" :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag>
     </div>
     <a-divider class="my-3"/>
     <div class="grid grid-cols-2 gap-3 text-sm">
      <div><div class="text-gray-500">商品金额</div><div class="mt-1 font-semibold">{{(item.amountMinor/100).toFixed(2)}} {{item.currency}}</div></div>
      <div><div class="text-gray-500">链上应付</div><div class="mt-1 font-semibold">{{item.paymentAmount==='0.000000'?'-':item.paymentAmount+' USDT'}}</div></div>
      <div><div class="text-gray-500">Telegram 用户</div><div class="mt-1 break-all">{{item.telegramUserId}}</div></div>
      <div><div class="text-gray-500">创建时间</div><div class="mt-1 break-words">{{formatTime(item.createdAt)}}</div></div>
     </div>
     <div v-if="item.deliveryError" class="mt-3 rounded-md bg-red-50 p-3 text-xs text-red-700 break-words">{{item.deliveryError}}</div>
     <div class="mt-3 flex flex-wrap items-center justify-between gap-2">
      <a-tag :color="item.stockReserved?'processing':'default'">{{item.stockReserved?'已预留库存':'未预留库存'}}</a-tag>
      <a-button v-if="item.status==='paid'||item.status==='delivery_failed'" type="primary" size="small" :loading="retryingId===item.id" @click="retryDelivery(item)">重试发货</a-button>
     </div>
     <div v-if="item.paymentReference" class="mt-2 truncate text-xs text-gray-500"><a :href="`https://tronscan.org/#/transaction/${item.paymentReference}`" target="_blank" rel="noreferrer">查看链上交易</a></div>
    </a-card>
    <a-empty v-if="!loading&&orders.length===0" description="暂无订单数据"/>
   </div>
   <a-table v-else :data-source="orders" :loading="loading" row-key="id" :scroll="{x:1100}">
    <a-table-column title="订单号" data-index="orderNo" key="orderNo" :width="190"/>
    <a-table-column title="商品" data-index="productName" key="productName"/>
    <a-table-column title="商品金额" key="amount" :width="130"><template #default="{record}">{{(record.amountMinor/100).toFixed(2)}} {{record.currency}}</template></a-table-column>
    <a-table-column title="链上应付金额" key="paymentAmount" :width="150"><template #default="{record}">{{record.paymentAmount==='0.000000'?'-':record.paymentAmount+' USDT'}}</template></a-table-column>
    <a-table-column title="支付方式" key="paymentMethod" :width="120"><template #default="{record}">{{record.paymentMethod==='trc20-usdt'?'TRC20 USDT':record.paymentMethod||'-'}}</template></a-table-column>
    <a-table-column title="链上交易" key="paymentReference" :width="190"><template #default="{record}"><a v-if="record.paymentReference" :href="`https://tronscan.org/#/transaction/${record.paymentReference}`" target="_blank" rel="noreferrer">{{record.paymentReference.slice(0,12)}}…</a><span v-else>-</span></template></a-table-column>
    <a-table-column title="Telegram 用户" data-index="telegramUserId" key="telegramUserId" :width="150"/>
    <a-table-column title="订单状态" key="status" :width="110"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="库存预留" key="stockReserved" :width="100"><template #default="{record}"><a-tag :color="record.stockReserved?'processing':'default'">{{record.stockReserved?'已预留':'未预留'}}</a-tag></template></a-table-column>
    <a-table-column title="发货重试" key="delivery" :width="120"><template #default="{record}"><a-button v-if="record.status==='paid'||record.status==='delivery_failed'" type="link" size="small" :loading="retryingId===record.id" @click="retryDelivery(record)">重试发货</a-button><span v-else>-</span></template></a-table-column>
    <a-table-column title="发货尝试次数" data-index="deliveryAttempts" key="deliveryAttempts" :width="120"/>
    <a-table-column title="最近发货错误" data-index="deliveryError" key="deliveryError" :width="220" ellipsis/>
    <a-table-column title="创建时间" key="createdAt" :width="190"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
    <a-table-column title="付款截止" key="expiresAt" :width="190"><template #default="{record}">{{formatTime(record.expiresAt)}}</template></a-table-column>
   </a-table>
   <a-alert class="mt-4" type="info" show-icon message="TRC20 USDT 经 TronGrid 确认后自动核验，付款成功进入自动发货队列；失败时可在此重试。"/>
  </a-card>
 </div>
</template>
