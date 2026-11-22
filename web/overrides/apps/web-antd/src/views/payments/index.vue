<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { useWindowSize } from '@vueuse/core';
import { useVbenModal, VbenButton } from '@vben/common-ui';
import { useVbenForm } from '#/adapter/form';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';

interface OrderOption { id:string; orderNo:string; productName:string; status:string; paymentAmount:string; paymentReference?:string; }
interface PaymentTransaction {
 id:string;provider:string;transactionId:string;fromAddress:string;toAddress:string;amountAtomic:number;tokenDecimals:number;
 blockTimestampMs:number;status:'matched'|'unmatched'|'reconciliation_required'|string;orderId?:string;walletRechargeId?:string;reconciliationNote?:string;firstSeenAt:string;updatedAt:string;
}
const loading=ref(false),transactions=ref<PaymentTransaction[]>([]),orders=ref<OrderOption[]>([]);
const reconcileLoading=ref(false),selectedTransaction=ref<PaymentTransaction|null>(null),filterStatus=ref('all'),keyword=ref('');
const {width}=useWindowSize(),isMobile=computed(()=>width.value<768);
const unmatchedCount=computed(()=>transactions.value.filter(x=>x.status==='unmatched').length);
const reviewCount=computed(()=>transactions.value.filter(x=>x.status==='reconciliation_required').length);
const matchedCount=computed(()=>transactions.value.filter(x=>x.status==='matched').length);
const filtered=computed(()=>{
 const q=keyword.value.trim().toLowerCase();
 return transactions.value.filter(x=>(filterStatus.value==='all'||x.status===filterStatus.value)&&(!q||[x.transactionId,x.fromAddress,x.toAddress,x.orderId||'',x.walletRechargeId||'',x.reconciliationNote||''].join(' ').toLowerCase().includes(q)));
});
const reconcileCandidates=computed(()=>{
 const item=selectedTransaction.value;if(!item)return[];
 return orders.value.filter(order=>{
  const amount=Math.round(Number(order.paymentAmount||0)*1_000_000);
  const stateOK=['pending_payment','expired'].includes(order.status)||(order.paymentReference===item.transactionId&&['paid','delivering','delivery_failed','delivered'].includes(order.status));
  return stateOK&&amount===item.amountAtomic;
 });
});
const [ReconcileModal,reconcileModalApi]=useVbenModal({title:$t('tg-card.payments.reconcileTitle'),onConfirm:submitReconciliation});
const [ReconcileForm,reconcileFormApi]=useVbenForm({
 commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',
 schema:[
  {component:'Select',fieldName:'orderId',label:$t('tg-card.payments.matchOrder'),componentProps:{options:computed(()=>reconcileCandidates.value.map(o=>({label:o.orderNo+' · '+o.productName+' · '+o.status,value:o.id}))),showSearch:true,optionFilterProp:'label',placeholder:$t('tg-card.payments.selectOrder')}},
  {component:'Textarea',fieldName:'note',label:$t('tg-card.payments.noteLabel'),componentProps:{rows:3,maxLength:1000,showCount:true,placeholder:$t('tg-card.payments.notePlaceholder')}},
 ],
});
function statusLabel(value:string){const key='tg-card.payments.status.'+value;const result=$t(key);return result===key?value:result}
function statusColor(value:string){if(value==='matched')return 'success';if(value==='reconciliation_required')return 'error';if(value==='unmatched')return 'warning';return 'default'}
function formatAmount(item:PaymentTransaction){return (item.amountAtomic/10**item.tokenDecimals).toFixed(item.tokenDecimals)}
function formatTime(value:number|string){const d=new Date(value);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
function shortAddress(value:string){return value?value.slice(0,8)+'…'+value.slice(-6):'-'}
function copy(value:string){if(value)navigator.clipboard?.writeText(value).then(()=>message.success($t('tg-card.payments.copied'))).catch(()=>message.error($t('tg-card.payments.copyFailed')))}
async function loadOrders(){try{orders.value=await requestClient.get<OrderOption[]>('/orders')}catch{message.error($t('tg-card.payments.ordersLoadFailed'))}}
async function loadTransactions(){loading.value=true;try{transactions.value=await requestClient.get<PaymentTransaction[]>('/payment/transactions')}catch{message.error($t('tg-card.payments.transactionsLoadFailed'))}finally{loading.value=false}}
async function openReconcile(item:PaymentTransaction){selectedTransaction.value=item;await loadOrders();await reconcileFormApi.setValues({orderId:item.orderId||'',note:item.reconciliationNote||''});reconcileModalApi.open()}
async function submitReconciliation(){
 const {valid}=await reconcileFormApi.validate();if(!valid)return;
 const values=await reconcileFormApi.getValues();if(!selectedTransaction.value||!values.orderId){message.warning($t('tg-card.payments.chooseOrder'));return}
 reconcileLoading.value=true;
 try{const path='/api/payment/transactions/'+selectedTransaction.value.transactionId+'/reconcile';const payload={orderId:values.orderId,note:values.note||''};await withAdminActionConfirmation({actionKey:'payment.transaction.reconcile',method:'POST',path,payload,run:(token)=>requestClient.post('/payment/transactions/'+selectedTransaction.value!.transactionId+'/reconcile',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success($t('tg-card.payments.reconcileSuccess'));reconcileModalApi.close();await Promise.all([loadTransactions(),loadOrders()])}
 catch{message.error($t('tg-card.payments.reconcileFailed'))}
 finally{reconcileLoading.value=false}
}
onMounted(loadTransactions);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-4 md:gap-4">
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.payments.chainTransactions')}}</div><div class="mt-2 text-2xl font-bold">{{transactions.length}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:bank-transfer" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.payments.matched')}}</div><div class="mt-2 text-2xl font-bold">{{matchedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:check-circle-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.payments.unmatched')}}</div><div class="mt-2 text-2xl font-bold">{{unmatchedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:alert-circle-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.payments.review')}}</div><div class="mt-2 text-2xl font-bold">{{reviewCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:clipboard-alert-outline" class="block text-2xl" /></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3 px-1 py-1"><div class="min-w-0"><div class="text-base font-semibold leading-6">{{$t('tg-card.payments.title')}}</div><div class="mt-1 text-xs leading-5 text-gray-500">{{$t('tg-card.payments.subtitle')}}</div></div><a-button size="small" class="mr-1" :loading="loading" @click="loadTransactions">{{$t('tg-card.payments.refresh')}}</a-button></div></template>
   <a-alert v-if="reviewCount>0" class="mb-4 rounded-lg border-2 border-red-300 bg-red-50" type="error" show-icon :message="$t('tg-card.payments.reviewAlert',{count:reviewCount})"  :description="$t('tg-card.payments.reviewDescription')" />
   <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-[180px_minmax(0,1fr)]">
    <a-select v-model:value="filterStatus" class="w-full"><a-select-option value="all">{{$t('tg-card.payments.all')}}</a-select-option><a-select-option value="matched">{{$t('tg-card.payments.matchedOrders')}}</a-select-option><a-select-option value="unmatched">{{$t('tg-card.payments.unmatched')}}</a-select-option><a-select-option value="reconciliation_required">{{$t('tg-card.payments.review')}}</a-select-option></a-select>
    <a-input v-model:value="keyword" allow-clear class="w-full" :placeholder="$t('tg-card.payments.search')"/>
   </div>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in filtered" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0 flex-1"><a class="block truncate font-semibold" :href="`https://tronscan.org/#/transaction/${item.transactionId}`" target="_blank" rel="noreferrer">{{item.transactionId}}</a><div class="mt-1 text-xs text-gray-500">{{$t('tg-card.payments.payment')}} {{shortAddress(item.fromAddress)}} · {{$t('tg-card.payments.receiving')}} {{shortAddress(item.toAddress)}}</div></div><a-tag class="shrink-0" :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag></div>
     <div class="mt-3 flex items-end justify-between gap-3"><div><div class="text-xs text-gray-500">{{$t('tg-card.payments.amountReceived')}}</div><div class="text-lg font-bold">{{formatAmount(item)}} USDT</div><div class="text-xs text-gray-500">{{formatTime(item.blockTimestampMs)}}</div></div><a-space wrap><a-button size="small" @click="copy(item.transactionId)">{{$t('tg-card.payments.copyHash')}}</a-button><a-button v-if="item.status!=='matched'" size="small" type="primary" @click="openReconcile(item)">{{$t('tg-card.payments.manualReconcile')}}</a-button></a-space></div>
     <div v-if="item.orderId" class="mt-2 break-all text-xs text-gray-500">{{$t('tg-card.payments.relatedOrder')}}{{item.orderId}}</div><div v-if="item.walletRechargeId" class="mt-2 break-all text-xs text-gray-500">{{$t('tg-card.payments.relatedRecharge')}}{{item.walletRechargeId}}</div>
     <div v-if="item.reconciliationNote" class="mt-2 break-words rounded-md bg-blue-50 p-2 text-xs text-blue-700">{{item.reconciliationNote}}</div>
    </a-card>
    <a-empty v-if="!loading&&filtered.length===0" :description="$t('tg-card.payments.empty')"/>
   </div>
   <a-table v-else :data-source="filtered" :loading="loading" row-key="id" :pagination="{pageSize:15,showSizeChanger:true}" :scroll="{x:1250}">
    <a-table-column :title="$t('tg-card.payments.txHash')" key="transactionId" :width="240" fixed="left"><template #default="{record}"><a :href="`https://tronscan.org/#/transaction/${record.transactionId}`" target="_blank" rel="noreferrer">{{record.transactionId.slice(0,12)}}…</a><a-button type="text" size="small" @click="copy(record.transactionId)">{{$t('tg-card.payments.copyHash')}}</a-button></template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.amountReceived')" key="amount" :width="150"><template #default="{record}">{{formatAmount(record)}} USDT</template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.from')" key="fromAddress" :width="170"><template #default="{record}">{{shortAddress(record.fromAddress)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.to')" key="toAddress" :width="170"><template #default="{record}">{{shortAddress(record.toAddress)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.confirmedAt')" key="blockTimestampMs" :width="190"><template #default="{record}">{{formatTime(record.blockTimestampMs)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.statusTitle')" key="status" :width="140"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.businessOrder')" key="businessOrder" :width="220"><template #default="{record}"><span v-if="record.orderId">{{$t('tg-card.payments.relatedOrder')}}{{record.orderId}}</span><span v-else-if="record.walletRechargeId">{{$t('tg-card.payments.relatedRecharge')}}{{record.walletRechargeId}}</span><span v-else>-</span></template></a-table-column>
    <a-table-column :title="$t('tg-card.payments.note')" data-index="reconciliationNote" key="reconciliationNote" :width="260" ellipsis/>
    <a-table-column :title="$t('tg-card.payments.actions')" key="actions" :width="130" fixed="right"><template #default="{record}"><a-button v-if="record.status!=='matched'" type="link" size="small" @click="openReconcile(record)">{{$t('tg-card.payments.linkOrReconcile')}}</a-button><span v-else>{{$t('tg-card.payments.processed')}}</span></template></a-table-column>
   </a-table>
   
  </a-card>
  <ReconcileModal class="w-full max-w-xl" :confirm-loading="reconcileLoading">
   <div class="p-4 sm:p-6">
    <a-alert class="mb-3" type="warning" show-icon :message="$t('tg-card.payments.confirmAlert')"/>
    <div v-if="selectedTransaction" class="mb-3 break-all rounded-lg bg-gray-50 p-3 text-sm">{{$t('tg-card.payments.amountDue')}}<strong>{{formatAmount(selectedTransaction)}} USDT</strong><div class="mt-1">{{$t('tg-card.payments.transaction')}}{{selectedTransaction.transactionId}}</div><div v-if="selectedTransaction.walletRechargeId" class="mt-1 text-red-600">{{$t('tg-card.payments.rechargeNeedsManual',{id:selectedTransaction.walletRechargeId})}}</div></div>
    <ReconcileForm/>
   </div>
  </ReconcileModal>
 </div>
</template>
