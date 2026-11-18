<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { useWindowSize } from '@vueuse/core';
import { useVbenModal, VbenButton } from '@vben/common-ui';
import { useVbenForm } from '#/adapter/form';
import { requestClient } from '#/api/request';

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
const [ReconcileModal,reconcileModalApi]=useVbenModal({title:'人工核对链上交易',onConfirm:submitReconciliation});
const [ReconcileForm,reconcileFormApi]=useVbenForm({
 commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',
 schema:[
  {component:'Select',fieldName:'orderId',label:'匹配订单（金额必须完全一致）',componentProps:{options:computed(()=>reconcileCandidates.value.map(o=>({label:o.orderNo+' · '+o.productName+' · '+o.status,value:o.id}))),showSearch:true,optionFilterProp:'label',placeholder:'选择订单'}},
  {component:'Textarea',fieldName:'note',label:'核对备注',componentProps:{rows:3,maxLength:1000,showCount:true,placeholder:'填写人工核对依据（选填）'}},
 ],
});
function statusLabel(value:string){return ({matched:'已匹配订单/充值',unmatched:'未匹配',reconciliation_required:'需要人工核对'} as Record<string,string>)[value]||value}
function statusColor(value:string){if(value==='matched')return 'success';if(value==='reconciliation_required')return 'error';if(value==='unmatched')return 'warning';return 'default'}
function formatAmount(item:PaymentTransaction){return (item.amountAtomic/10**item.tokenDecimals).toFixed(item.tokenDecimals)}
function formatTime(value:number|string){const d=new Date(value);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
function shortAddress(value:string){return value?value.slice(0,8)+'…'+value.slice(-6):'-'}
function copy(value:string){if(value)navigator.clipboard?.writeText(value).then(()=>message.success('已复制')).catch(()=>message.error('复制失败'))}
async function loadOrders(){try{orders.value=await requestClient.get<OrderOption[]>('/orders')}catch{message.error('订单加载失败，无法人工补单')}}
async function loadTransactions(){loading.value=true;try{transactions.value=await requestClient.get<PaymentTransaction[]>('/payment/transactions')}catch{message.error('支付流水加载失败')}finally{loading.value=false}}
async function openReconcile(item:PaymentTransaction){selectedTransaction.value=item;await loadOrders();await reconcileFormApi.setValues({orderId:item.orderId||'',note:item.reconciliationNote||''});reconcileModalApi.open()}
async function submitReconciliation(){
 const {valid}=await reconcileFormApi.validate();if(!valid)return;
 const values=await reconcileFormApi.getValues();if(!selectedTransaction.value||!values.orderId){message.warning('请选择匹配订单');return}
 reconcileLoading.value=true;
 try{await requestClient.post('/payment/transactions/'+selectedTransaction.value.transactionId+'/reconcile',{orderId:values.orderId,note:values.note||''});message.success('交易核对完成，系统已自动尝试发货');reconcileModalApi.close();await Promise.all([loadTransactions(),loadOrders()])}
 catch{message.error('人工核对失败，请检查金额、收款地址、订单状态及库存')}
 finally{reconcileLoading.value=false}
}
onMounted(loadTransactions);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-4 md:gap-4">
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">链上流水</div><div class="mt-2 text-2xl font-bold">{{transactions.length}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:bank-transfer" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">已匹配</div><div class="mt-2 text-2xl font-bold">{{matchedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:check-circle-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">未匹配</div><div class="mt-2 text-2xl font-bold">{{unmatchedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:alert-circle-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">需人工核对</div><div class="mt-2 text-2xl font-bold">{{reviewCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:clipboard-alert-outline" class="block text-2xl" /></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3 px-1 py-1"><div class="min-w-0"><div class="text-base font-semibold leading-6">支付风险控制中心</div><div class="mt-1 text-xs leading-5 text-gray-500">自动识别重复交易、金额异常、过期付款和未匹配到账；异常交易需要人工确认</div></div><a-button size="small" class="mr-1" :loading="loading" @click="loadTransactions">刷新流水</a-button></div></template>
   <a-alert v-if="reviewCount>0" class="mb-4 rounded-lg border-2 border-red-300 bg-red-50" type="error" show-icon :message="'需要立即人工核对：'+reviewCount+' 笔链上交易'" description="请优先检查交易金额、收款地址和对应订单，完成核对前不要手动发货或重复处理。" />
   <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-[180px_minmax(0,1fr)]">
    <a-select v-model:value="filterStatus" class="w-full"><a-select-option value="all">全部状态</a-select-option><a-select-option value="matched">已匹配订单</a-select-option><a-select-option value="unmatched">未匹配</a-select-option><a-select-option value="reconciliation_required">需要人工核对</a-select-option></a-select>
    <a-input v-model:value="keyword" allow-clear class="w-full" placeholder="搜索交易哈希、地址、订单 ID 或核对备注"/>
   </div>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in filtered" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0 flex-1"><a class="block truncate font-semibold" :href="`https://tronscan.org/#/transaction/${item.transactionId}`" target="_blank" rel="noreferrer">{{item.transactionId}}</a><div class="mt-1 text-xs text-gray-500">付款 {{shortAddress(item.fromAddress)}} · 收款 {{shortAddress(item.toAddress)}}</div></div><a-tag class="shrink-0" :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag></div>
     <div class="mt-3 flex items-end justify-between gap-3"><div><div class="text-xs text-gray-500">到账金额</div><div class="text-lg font-bold">{{formatAmount(item)}} USDT</div><div class="text-xs text-gray-500">{{formatTime(item.blockTimestampMs)}}</div></div><a-space wrap><a-button size="small" @click="copy(item.transactionId)">复制哈希</a-button><a-button v-if="item.status!=='matched'" size="small" type="primary" @click="openReconcile(item)">人工补单</a-button></a-space></div>
     <div v-if="item.orderId" class="mt-2 break-all text-xs text-gray-500">关联订单：{{item.orderId}}</div><div v-if="item.walletRechargeId" class="mt-2 break-all text-xs text-gray-500">关联充值单：{{item.walletRechargeId}}</div>
     <div v-if="item.reconciliationNote" class="mt-2 break-words rounded-md bg-blue-50 p-2 text-xs text-blue-700">{{item.reconciliationNote}}</div>
    </a-card>
    <a-empty v-if="!loading&&filtered.length===0" description="暂无链上交易流水"/>
   </div>
   <a-table v-else :data-source="filtered" :loading="loading" row-key="id" :pagination="{pageSize:15,showSizeChanger:true}" :scroll="{x:1250}">
    <a-table-column title="链上交易哈希" key="transactionId" :width="240" fixed="left"><template #default="{record}"><a :href="`https://tronscan.org/#/transaction/${record.transactionId}`" target="_blank" rel="noreferrer">{{record.transactionId.slice(0,12)}}…</a><a-button type="text" size="small" @click="copy(record.transactionId)">复制</a-button></template></a-table-column>
    <a-table-column title="到账金额" key="amount" :width="150"><template #default="{record}">{{formatAmount(record)}} USDT</template></a-table-column>
    <a-table-column title="付款地址" key="fromAddress" :width="170"><template #default="{record}">{{shortAddress(record.fromAddress)}}</template></a-table-column>
    <a-table-column title="收款地址" key="toAddress" :width="170"><template #default="{record}">{{shortAddress(record.toAddress)}}</template></a-table-column>
    <a-table-column title="链上确认时间" key="blockTimestampMs" :width="190"><template #default="{record}">{{formatTime(record.blockTimestampMs)}}</template></a-table-column>
    <a-table-column title="处理状态" key="status" :width="140"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="关联业务单" key="businessOrder" :width="220"><template #default="{record}"><span v-if="record.orderId">订单：{{record.orderId}}</span><span v-else-if="record.walletRechargeId">充值：{{record.walletRechargeId}}</span><span v-else>-</span></template></a-table-column>
    <a-table-column title="核对备注" data-index="reconciliationNote" key="reconciliationNote" :width="260" ellipsis/>
    <a-table-column title="操作" key="actions" :width="130" fixed="right"><template #default="{record}"><a-button v-if="record.status!=='matched'" type="link" size="small" @click="openReconcile(record)">人工关联 / 补单</a-button><span v-else>已处理</span></template></a-table-column>
   </a-table>
   <a-alert class="mt-4" type="info" show-icon message="人工补单校验收款地址、交易金额、订单创建时间和库存；退款功能尚未开放。"/>
  </a-card>
  <ReconcileModal class="w-full max-w-xl" :confirm-loading="reconcileLoading">
   <div class="p-4 sm:p-6">
    <a-alert class="mb-3" type="warning" show-icon message="确认后系统将标记订单已付款并尝试自动发货，请先核对交易和金额。"/>
    <div v-if="selectedTransaction" class="mb-3 break-all rounded-lg bg-gray-50 p-3 text-sm">到账：<strong>{{formatAmount(selectedTransaction)}} USDT</strong><div class="mt-1">交易：{{selectedTransaction.transactionId}}</div><div v-if="selectedTransaction.walletRechargeId" class="mt-1 text-red-600">充值单：{{selectedTransaction.walletRechargeId}}（需人工核对后到用户管理调整余额）</div></div>
    <ReconcileForm/>
   </div>
  </ReconcileModal>
 </div>
</template>
