<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { useWindowSize } from '@vueuse/core';
import { useRoute } from 'vue-router';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';

interface Order {
  id: string; orderNo: string; botId: string; telegramUserId: string; telegramChatId: string;
  productName: string; unitPriceMinor: number; quantity: number; amountMinor: number;
  paymentAmount: string; paymentMethod?: string; paymentReference?: string; currency: string;
  status: string; expiresAt: string; createdAt: string; paidAt?: string; deliveredAt?: string;
  deliveryAttempts: number; deliveryError?: string; stockReserved: boolean; deliveryType?: string;
}
interface DeliveryStep { id:string; orderId:string; stepKey:string; stepOrder:number; status:string; attempts:number; telegramMessageId:string; lastError:string; startedAt:string; completedAt:string; createdAt:string; updatedAt:string; }
interface OrderDetail { order: Order; deliverySteps:DeliveryStep[]; paymentFromAddress:string; paymentToAddress:string; paymentTransactionId:string; paymentBlockTimestampMs:number; }
const loading=ref(false), orders=ref<Order[]>([]), retryingId=ref(''), detailLoading=ref(false), detailOpen=ref(false), selectedDetail=ref<OrderDetail|null>(null);
const cleanupOpen=ref(false),cleanupPhrase=ref(''),cleanupLoading=ref(false);
const {width}=useWindowSize();
const route=useRoute();
const mobilePage=ref(1);
const mobilePageSize=10;
const mobileOrders=computed(()=>orders.value.slice((mobilePage.value-1)*mobilePageSize,mobilePage.value*mobilePageSize));
const isMobile=computed(()=>width.value<768);
const pendingCount=computed(()=>orders.value.filter(x=>x.status==='pending_payment').length);
const reservedCount=computed(()=>orders.value.filter(x=>x.stockReserved).length);
const failedCount=computed(()=>orders.value.filter(x=>x.status==='delivery_failed'||x.status==='delivery_uncertain').length);
function statusLabel(status:string){const key='tg-card.orders.status.'+status;const value=$t(key);return value===key?status:value}
function statusColor(status:string){if(status==='delivered')return 'success';if(status==='paid'||status==='delivering'||status==='procurement_pending')return 'processing';if(status==='delivery_failed'||status==='delivery_uncertain'||status==='procurement_failed')return 'error';if(status==='pending_payment')return 'warning';return 'default'}
function formatTime(value:string|number){if(!value)return '-';const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString()}
function copy(value:string){if(!value)return;navigator.clipboard?.writeText(value).then(()=>message.success($t('tg-card.orders.copied'))).catch(()=>message.error($t('tg-card.orders.copyFailed')))}
async function loadOrders(){loading.value=true;try{orders.value=await requestClient.get<Order[]>('/orders');mobilePage.value=1}catch{message.error($t('tg-card.orders.loadFailed'))}finally{loading.value=false}}
async function openOrderDetail(order:Order){detailOpen.value=true;detailLoading.value=true;selectedDetail.value=null;try{selectedDetail.value=await requestClient.get<OrderDetail>('/orders/'+order.id)}catch{message.error($t('tg-card.orders.detailFailed'))}finally{detailLoading.value=false}}
async function retryDelivery(order:Order){
 if(order.status==='delivery_uncertain'){
  const prompt=order.deliveryType==='file_card'
   ? $t('tg-card.orders.uncertainFileConfirm')
   : $t('tg-card.orders.uncertainConfirm');
  if(!window.confirm(prompt))return;
 }
 retryingId.value=order.id;
 try{const path='/api/orders/'+order.id+'/retry-delivery';await withAdminActionConfirmation({actionKey:'order.delivery.retry',method:'POST',path,run:(token)=>requestClient.post('/orders/'+order.id+'/retry-delivery',undefined,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success($t('tg-card.orders.retryTriggered',{orderNo:order.orderNo}));await loadOrders();if(selectedDetail.value?.order.id===order.id)await openOrderDetail(order)}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.orders.retryFailed'));await loadOrders();if(selectedDetail.value?.order.id===order.id)await openOrderDetail(order)}
 finally{retryingId.value=''}
}
function deliveryStepLabel(key:string){const value=$t('tg-card.orders.step.'+key);return value==='tg-card.orders.step.'+key?key:value}
function deliveryStepStatusLabel(status:string){const value=$t('tg-card.orders.stepStatus.'+status);return value==='tg-card.orders.stepStatus.'+status?status:value}
function deliveryStepStatusColor(status:string){return ({succeeded:'success',failed:'error',uncertain:'warning',sending:'processing',pending:'default'} as Record<string,string>)[status]||'default'}
async function resolveDeliveryStep(step:DeliveryStep,status:'succeeded'|'failed'){
 const note=window.prompt(status==='succeeded'?$t('tg-card.orders.verifySuccessPrompt'):$t('tg-card.orders.verifyFailedPrompt'));
 if(!note?.trim())return;
 let messageId='';
 if(status==='succeeded')messageId=window.prompt($t('tg-card.orders.messageIdPrompt'))?.trim()||'';
 try{
  const path='/api/orders/'+step.orderId+'/delivery-steps/'+encodeURIComponent(step.stepKey);const payload={status,messageId,note:note.trim()};await withAdminActionConfirmation({actionKey:'order.delivery.resolve-step',method:'PUT',path,payload,run:(token)=>requestClient.put('/orders/'+step.orderId+'/delivery-steps/'+encodeURIComponent(step.stepKey),payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});
  message.success($t('tg-card.orders.verifySaved'));if(selectedDetail.value)await openOrderDetail(selectedDetail.value.order);await loadOrders();
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.orders.verifyFailed'))}
}
async function clearAllOrderData(){if(cleanupPhrase.value.trim()!==$t('tg-card.orders.cleanupPhrase')){message.warning($t('tg-card.orders.cleanupPrompt'));return;}cleanupLoading.value=true;try{const path='/api/orders/cleanup';const payload={confirmation:'CLEAR_ALL_ORDERS'};const result=await withAdminActionConfirmation({actionKey:'order.cleanup',method:'POST',path,payload,run:(token)=>requestClient.post<{orders:number;paymentTransactions:number;supportTickets:number;walletTransactions:number;walletRecharges:number}>('/orders/cleanup',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});if(!result)return;message.success($t('tg-card.orders.cleanupDone',{orders:result.orders,payments:result.paymentTransactions,recharges:result.walletRecharges}));cleanupOpen.value=false;cleanupPhrase.value='';await loadOrders();}catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.orders.cleanupFailed'));}finally{cleanupLoading.value=false;}}
onMounted(async()=>{await loadOrders();const orderNo=String(route.query.orderNo||'');if(orderNo){const target=orders.value.find(item=>item.orderNo===orderNo);if(target)await openOrderDetail(target);}});
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-4 md:gap-4">
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500"{{$t('tg-card.orders.total')}}div><div class="mt-2 text-2xl font-bold">{{orders.length}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:receipt-text-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500"{{$t('tg-card.orders.pending')}}div><div class="mt-2 text-2xl font-bold">{{pendingCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:clock-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500"{{$t('tg-card.orders.reserved')}}div><div class="mt-2 text-2xl font-bold">{{reservedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:package-variant-closed" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500"{{$t('tg-card.orders.deliveryIssues')}}div><div class="mt-2 text-2xl font-bold">{{failedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:alert-circle-outline" class="block text-2xl" /></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3 px-1 py-1"><div class="min-w-0"><div class="text-base font-semibold leading-6"{{$t('tg-card.orders.title')}}div><div class="mt-1 text-xs leading-5 text-gray-500"{{$t('tg-card.orders.subtitle')}}div></div><div class="flex flex-wrap items-center gap-3"><a-button size="small" danger ghost @click="cleanupOpen=true"{{$t('tg-card.orders.cleanup')}}a-button><a-button size="small" :loading="loading" @click="loadOrders"{{$t('tg-card.orders.refresh')}}a-button></div></div></template>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in mobileOrders" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
       <div class="flex min-w-0 items-center gap-2"><span class="truncate font-semibold">{{item.orderNo}}</span><a-button size="small" type="text" @click="copy(item.orderNo)"{{$t('tg-card.orders.copy')}}a-button></div>
       <div class="mt-1 break-words text-sm text-gray-600">{{item.productName}}</div>
      </div>
      <a-tag class="shrink-0" :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag>
     </div>
     <a-divider class="my-3"/>
     <div class="grid grid-cols-2 gap-3 text-sm">
      <div><div class="text-gray-500"{{$t('tg-card.orders.productAmount')}}div><div class="mt-1 font-semibold">{{(item.amountMinor/100).toFixed(2)}} {{item.currency}}</div></div>
      <div><div class="text-gray-500"{{$t('tg-card.orders.onchainDue')}}div><div class="mt-1 font-semibold">{{item.paymentAmount==='0.000000'?'-':item.paymentAmount+' USDT'}}</div></div>
      <div><div class="text-gray-500"{{$t('tg-card.orders.telegramUser')}}div><div class="mt-1 break-all">{{item.telegramUserId}}</div></div>
      <div><div class="text-gray-500"{{$t('tg-card.orders.createdAt')}}div><div class="mt-1 break-words">{{formatTime(item.createdAt)}}</div></div>
     </div>
     <div v-if="item.deliveryError" class="mt-3 rounded-md bg-red-50 p-3 text-xs text-red-700 break-words">{{item.deliveryError}}</div>
     <div class="mt-3 flex flex-wrap items-center justify-between gap-2">
      <a-button size="small" @click="openOrderDetail(item)"{{$t('tg-card.orders.details')}}a-button>
      <a-tag :color="item.stockReserved?'processing':'default'">{{item.stockReserved?'已预留库存':'未预留库存'}}</a-tag>
      <a-button v-if="item.status==='paid'||item.status==='delivery_failed'||(item.status==='delivery_uncertain'&&item.deliveryType!=='invite_link')" type="primary" size="small" :loading="retryingId===item.id" @click="retryDelivery(item)"{{$t('tg-card.orders.retryDelivery')}}a-button>
      <a-tooltip v-else-if="item.status==='delivery_uncertain'&&item.deliveryType==='invite_link'" title="一次性邀请链接的结果不确定，禁止自动重新创建；请人工核实并处理。"><span class="text-xs text-orange-600"{{$t('tg-card.orders.manualVerify')}}span></a-tooltip>
     </div>
     <div v-if="item.paymentReference" class="mt-2 truncate text-xs text-gray-500"><a :href="`https://tronscan.org/#/transaction/${item.paymentReference}`" target="_blank" rel="noreferrer"{{$t('tg-card.orders.viewChain')}}a></div>
    </a-card>
    <a-empty v-if="!loading&&orders.length===0" :description="$t('tg-card.orders.empty')"/>
    <div v-if="orders.length>mobilePageSize" class="flex justify-center py-3"><a-pagination v-model:current="mobilePage" :page-size="mobilePageSize" :total="orders.length" size="small" show-less-items /></div>
   </div>
   <a-table v-else :data-source="orders" :loading="loading" row-key="id" :pagination="{pageSize:10,showSizeChanger:true}" :scroll="{x:1100}">
    <a-table-column :title="$t('tg-card.orders.orderNo')" key="orderNo" :width="220"><template #default="{record}"><a-button type="link" size="small" class="p-0" @click="openOrderDetail(record)">{{record.orderNo}}</a-button></template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.product')" data-index="productName" key="productName"/>
    <a-table-column :title="$t('tg-card.orders.productAmount')" key="amount" :width="130"><template #default="{record}">{{(record.amountMinor/100).toFixed(2)}} {{record.currency}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.onchainAmount')" key="paymentAmount" :width="150"><template #default="{record}">{{record.paymentAmount==='0.000000'?'-':record.paymentAmount+' USDT'}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.paymentMethod')" key="paymentMethod" :width="120"><template #default="{record}">{{record.paymentMethod==='trc20-usdt'?'TRC20 USDT':record.paymentMethod||'-'}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.chainTransaction')" key="paymentReference" :width="190"><template #default="{record}"><a v-if="record.paymentReference" :href="`https://tronscan.org/#/transaction/${record.paymentReference}`" target="_blank" rel="noreferrer">{{record.paymentReference.slice(0,12)}}…</a><span v-else>-</span></template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.telegramUser')" data-index="telegramUserId" key="telegramUserId" :width="150"/>
    <a-table-column :title="$t('tg-card.orders.orderStatus')" key="status" :width="110"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.reserved')" key="stockReserved" :width="100"><template #default="{record}"><a-tag :color="record.stockReserved?'processing':'default'">{{record.stockReserved?'已预留':'未预留'}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.deliveryRetry')" key="delivery" :width="120"><template #default="{record}"><a-button v-if="record.status==='paid'||record.status==='delivery_failed'||(record.status==='delivery_uncertain'&&record.deliveryType!=='invite_link')" type="link" size="small" :loading="retryingId===record.id" @click="retryDelivery(record)"{{$t('tg-card.orders.retryDelivery')}}a-button><a-tooltip v-else-if="record.status==='delivery_uncertain'&&record.deliveryType==='invite_link'" title="该交付包含可能已成功的外部步骤，禁止整单重试；请人工核实并处理。"><span class="text-xs text-orange-600"{{$t('tg-card.orders.manualVerify')}}span></a-tooltip><span v-else>-</span></template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.attempts')" data-index="deliveryAttempts" key="deliveryAttempts" :width="120"/>
    <a-table-column :title="$t('tg-card.orders.lastError')" data-index="deliveryError" key="deliveryError" :width="220" ellipsis/>
    <a-table-column :title="$t('tg-card.orders.createdAt')" key="createdAt" :width="190"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.orders.expiresAt')" key="expiresAt" :width="190"><template #default="{record}">{{formatTime(record.expiresAt)}}</template></a-table-column>
   </a-table>
   
  </a-card>
  <a-modal v-model:open="cleanupOpen" :title="$t('tg-card.orders.cleanupTitle')" :confirm-loading="cleanupLoading" :ok-text="$t('tg-card.orders.cleanupConfirm')" ok-type="danger" :cancel-text="$t('tg-card.orders.cancel')" @ok="clearAllOrderData" @cancel="cleanupPhrase=''">
   <a-alert class="mb-4" type="error" show-icon :message="$t('tg-card.orders.irreversible')" :description="$t('tg-card.orders.cleanupDescription')"/>
   <div class="mb-2 text-sm">{{$t('tg-card.orders.cleanupInput')}}</div><a-input v-model:value="cleanupPhrase" placeholder="清空全部订单数据"/>
  </a-modal>
  <a-modal v-model:open="detailOpen" title="订单详情" :footer="null" width="min(720px, 94vw)">
   <a-spin :spinning="detailLoading">
    <template v-if="selectedDetail">
     <div v-if="selectedDetail.deliverySteps?.length" class="mb-4 rounded-lg border border-gray-100 p-3 sm:col-span-2">
       <div class="mb-3 font-semibold"{{$t('tg-card.orders.deliverySteps')}}div>
       <div v-for="step in selectedDetail.deliverySteps" :key="step.id" class="mb-3 last:mb-0 rounded-md border border-gray-100 p-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
         <span class="font-medium">{{deliveryStepLabel(step.stepKey)}}</span>
         <a-tag :color="deliveryStepStatusColor(step.status)">{{deliveryStepStatusLabel(step.status)}}</a-tag>
        </div>
        <div class="mt-2 grid grid-cols-2 gap-2 text-xs text-gray-500">
         <div>尝试次数：{{step.attempts}}</div>
         <div>Telegram message_id：{{step.telegramMessageId||'-'}}</div>
         <div class="col-span-2 break-words" v-if="step.lastError">最近错误：{{step.lastError}}</div>
         <div class="col-span-2">最近开始：{{formatTime(step.startedAt||'')}}</div>
        </div>
        <div v-if="step.status==='uncertain' && selectedDetail.order.status==='delivery_uncertain'" class="mt-3 flex flex-wrap gap-2">
         <a-button size="small" type="primary" @click="resolveDeliveryStep(step,'succeeded')"{{$t('tg-card.orders.markSucceeded')}}a-button>
         <a-button size="small" danger @click="resolveDeliveryStep(step,'failed')"{{$t('tg-card.orders.markFailed')}}a-button>
        </div>
       </div>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.orderNumber')}}div><div class="mt-1 break-all font-semibold">{{selectedDetail.order.orderNo}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.orderStatus')}}div><div class="mt-1"><a-tag :color="statusColor(selectedDetail.order.status)">{{statusLabel(selectedDetail.order.status)}}</a-tag></div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.telegramUserId')}}div><div class="mt-1 break-all">{{selectedDetail.order.telegramUserId}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.telegramChatId')}}div><div class="mt-1 break-all">{{selectedDetail.order.telegramChatId}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.product')}}div><div class="mt-1">{{selectedDetail.order.productName}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.amountDue')}}div><div class="mt-1 font-semibold">{{selectedDetail.order.paymentAmount}} USDT</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.orderCreated')}}div><div class="mt-1">{{formatTime(selectedDetail.order.createdAt)}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.paymentConfirmed')}}div><div class="mt-1">{{formatTime(selectedDetail.order.paidAt||'')}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.chainReceived')}}div><div class="mt-1">{{selectedDetail.paymentBlockTimestampMs?formatTime(selectedDetail.paymentBlockTimestampMs):'-'}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3"><div class="text-xs text-gray-500"{{$t('tg-card.orders.deliveredAt')}}div><div class="mt-1">{{formatTime(selectedDetail.order.deliveredAt||'')}}</div></div>
      <div class="rounded-lg border border-gray-100 p-3 sm:col-span-2"><div class="text-xs text-gray-500"{{$t('tg-card.orders.payerAddress')}}div><div class="mt-1 break-all">{{selectedDetail.paymentFromAddress||'-'}} <a-button v-if="selectedDetail.paymentFromAddress" size="small" type="link" @click="copy(selectedDetail.paymentFromAddress)"{{$t('tg-card.orders.copy')}}a-button></div></div>
      <div class="rounded-lg border border-gray-100 p-3 sm:col-span-2"><div class="text-xs text-gray-500"{{$t('tg-card.orders.platformAddress')}}div><div class="mt-1 break-all">{{selectedDetail.paymentToAddress||'-'}} <a-button v-if="selectedDetail.paymentToAddress" size="small" type="link" @click="copy(selectedDetail.paymentToAddress)"{{$t('tg-card.orders.copy')}}a-button></div></div>
      <div class="rounded-lg border border-gray-100 p-3 sm:col-span-2"><div class="text-xs text-gray-500"{{$t('tg-card.orders.txHash')}}div><div class="mt-1 break-all">{{selectedDetail.paymentTransactionId||selectedDetail.order.paymentReference||'-'}}</div><a v-if="selectedDetail.paymentTransactionId" class="mt-1 inline-block" :href="'https://tronscan.org/#/transaction/'+selectedDetail.paymentTransactionId" target="_blank" rel="noreferrer"{{$t('tg-card.orders.viewTronscan')}}a></div>
     </div>
    </template>
   </a-spin>
  </a-modal>
 </div>
</template>
