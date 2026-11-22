<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface DailyReport {
 reportDate:string;revenueAtomic:number;orderCount:number;deliveredCount:number;deliveryFailedCount:number;
 refundAtomic:number;rechargeAtomic:number;unmatchedTransactionCount:number;manualReviewTransactionCount:number;supplierBalance:string;supplierCurrency:string;supplierBalanceCheckedAt:string;generatedAt:string;
}
interface BalanceRun {runKey:string;runAt:string;checkedUsers:number;mismatchUsers:number;totalDifferenceAtomic:number}
interface BalanceAnomaly {runKey:string;telegramUserId:string;recordedBalanceAtomic:number;ledgerBalanceAtomic:number;differenceAtomic:number;createdAt:string}
const month=ref(new Date().toISOString().slice(0,7));
const reports=ref<DailyReport[]>([]),runs=ref<BalanceRun[]>([]),anomalies=ref<BalanceAnomaly[]>([]);
const loading=ref(false),checking=ref(false),selectedRun=ref('');
const detailOpen=ref(false),detailLoading=ref(false);
const anomalyCount=computed(()=>runs.value[0]?.mismatchUsers||0);
function runLabel(value:string){return value.startsWith('manual-')?$t('tg-card.reconciliation.runManual')+value.slice(7):value}
function money(value:number){return (Number(value||0)/1_000_000).toFixed(6)+' USDT'}
function when(value:string){const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString()}
async function loadAnomalies(runKey:string, open=true){selectedRun.value=runKey;if(open)detailOpen.value=true;detailLoading.value=true;anomalies.value=[];try{anomalies.value=await requestClient.get<BalanceAnomaly[]>('/reconciliation/balance-anomalies',{params:{runKey}})}catch{message.error($t('tg-card.reconciliation.anomaliesFailed'))}finally{detailLoading.value=false}}
async function load(){
 loading.value=true;
 try{
  const [daily,history]=await Promise.all([
   requestClient.get<DailyReport[]>('/reconciliation/reports',{params:{month:month.value}}),
   requestClient.get<BalanceRun[]>('/reconciliation/balance-runs',{params:{limit:24}}),
  ]);
  reports.value=daily;runs.value=history;
  if(history.length>0){await loadAnomalies(selectedRun.value&&history.some(x=>x.runKey===selectedRun.value)?selectedRun.value:history[0].runKey,false)}
  else{anomalies.value=[];selectedRun.value=''}
 }catch{message.error($t('tg-card.reconciliation.loadFailed'))}finally{loading.value=false}
}
async function runCheck(){
 checking.value=true;
 try{
  const result=await requestClient.post<{run:BalanceRun;anomalies:BalanceAnomaly[]}>('/reconciliation/balance-check',{});
  message.success(result.run.mismatchUsers>0?$t('tg-card.reconciliation.checkMismatch'):$t('tg-card.reconciliation.checkConsistent'));
  await load();if(result.run.runKey)await loadAnomalies(result.run.runKey);
 }catch{message.error($t('tg-card.reconciliation.checkFailed'))}finally{checking.value=false}
}
const totals=computed(()=>reports.value.reduce((a,x)=>({orders:a.orders+x.orderCount,revenue:a.revenue+x.revenueAtomic,refund:a.refund+x.refundAtomic,recharge:a.recharge+x.rechargeAtomic,failed:a.failed+x.deliveryFailedCount,unmatched:a.unmatched+x.unmatchedTransactionCount,review:a.review+x.manualReviewTransactionCount}),{orders:0,revenue:0,refund:0,recharge:0,failed:0,unmatched:0,review:0}));
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-end justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">{{$t('tg-card.reconciliation.title')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.reconciliation.subtitle')}}</p></div>
   <div class="flex flex-wrap items-center gap-2"><a-date-picker v-model:value="month" picker="month" value-format="YYYY-MM" :placeholder="$t('tg-card.reconciliation.month')" class="w-40" @change="load"/><a-button size="small" :loading="loading" @click="load">{{$t('tg-card.reconciliation.refresh')}}</a-button><a-button size="small" type="primary" :loading="checking" @click="runCheck">{{$t('tg-card.reconciliation.runCheck')}}</a-button></div>
  </div>
  <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.reconciliation.orders')}}</div><div class="mt-2 text-2xl font-semibold">{{totals.orders}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.reconciliation.revenue')}}</div><div class="mt-2 text-xl font-semibold">{{money(totals.revenue)}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.reconciliation.refund')}}</div><div class="mt-2 text-xl font-semibold">{{money(totals.refund)}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.reconciliation.recharge')}}</div><div class="mt-2 text-xl font-semibold">{{money(totals.recharge)}}</div></a-card>
  </div>
  <a-card :bordered="false" :loading="loading">
   <template #title><div class="text-base font-semibold">{{$t('tg-card.reconciliation.calendar')}}{{month}}</div></template>
   <a-alert class="mb-3" type="info" show-icon :message="$t('tg-card.reconciliation.dailyNote')" />
   <a-table :data-source="reports" row-key="reportDate" :pagination="{pageSize:31,hideOnSinglePage:true}" :scroll="{x:1580}">
    <a-table-column :title="$t('tg-card.reconciliation.dateUTC')" data-index="reportDate" key="date" :width="130"/>
    <a-table-column :title="$t('tg-card.reconciliation.revenue')" key="revenue" :width="150"><template #default="{record}">{{money(record.revenueAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.orders')" data-index="orderCount" key="orders" :width="90"/>
    <a-table-column :title="$t('tg-card.reconciliation.delivered')" data-index="deliveredCount" key="delivered" :width="90"/>
    <a-table-column :title="$t('tg-card.reconciliation.deliveryFailed')" data-index="deliveryFailedCount" key="failed" :width="100"/>
    <a-table-column :title="$t('tg-card.reconciliation.refundAmount')" key="refund" :width="150"><template #default="{record}">{{money(record.refundAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.rechargeTotal')" key="recharge" :width="150"><template #default="{record}">{{money(record.rechargeAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.unmatched')" data-index="unmatchedTransactionCount" key="unmatched" :width="110"/>
    <a-table-column :title="$t('tg-card.reconciliation.manualReview')" data-index="manualReviewTransactionCount" key="review" :width="110"/>
    <a-table-column :title="$t('tg-card.reconciliation.supplierBalance')" key="supplierBalance" :width="160"><template #default="{record}">{{record.supplierBalance?record.supplierBalance+' '+record.supplierCurrency:$t('tg-card.reconciliation.notFetched')}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.balanceChecked')" key="supplierBalanceCheckedAt" :width="190"><template #default="{record}">{{record.supplierBalanceCheckedAt?when(record.supplierBalanceCheckedAt):'—'}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.generated')" key="generated" :width="190"><template #default="{record}">{{when(record.generatedAt)}}</template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&reports.length===0" :description="$t('tg-card.reconciliation.noReports')"/>
  </a-card>
  <a-card :bordered="false" :loading="loading">
   <template #title><div class="flex flex-wrap items-center justify-between gap-2"><span class="text-base font-semibold">{{$t('tg-card.reconciliation.consistency')}}</span><a-tag :color="anomalyCount>0?'error':'success'">{{anomalyCount>0?$t('tg-card.reconciliation.latestAnomaly'):$t('tg-card.reconciliation.latestClean')}}</a-tag></div></template>
   <a-alert class="mb-3" type="warning" show-icon :message="$t('tg-card.reconciliation.readOnlyNote')" />
   <a-table :data-source="runs" row-key="runKey" :pagination="{pageSize:8,hideOnSinglePage:true}" :scroll="{x:800}">
    <a-table-column :title="$t('tg-card.reconciliation.checkTime')" key="time" :width="190"><template #default="{record}">{{when(record.runAt)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.checkedUsers')" data-index="checkedUsers" key="checked" :width="110"/>
    <a-table-column :title="$t('tg-card.reconciliation.anomalyUsers')" data-index="mismatchUsers" key="mismatch" :width="110"/>
    <a-table-column :title="$t('tg-card.reconciliation.totalDifference')" key="diff" :width="180"><template #default="{record}">{{money(record.totalDifferenceAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.reconciliation.actions')" key="action" :width="120"><template #default="{record}"><a-button size="small" @click="loadAnomalies(record.runKey)">{{$t('tg-card.reconciliation.viewDetails')}}</a-button></template></a-table-column>
   </a-table>

  </a-card>
  <a-modal v-model:open="detailOpen" :title="$t('tg-card.reconciliation.detailTitle')" :footer="null" width="900px">
   <div class="mb-3 text-xs text-gray-500">{{$t('tg-card.reconciliation.run')}}{{runLabel(selectedRun)}}</div>
   <a-spin :spinning="detailLoading">
    <a-table :data-source="anomalies" row-key="telegramUserId" :pagination="{pageSize:10,hideOnSinglePage:true}" :scroll="{x:850}">
     <a-table-column :title="$t('tg-card.reconciliation.telegramUser')" data-index="telegramUserId" key="user" :width="180"/>
     <a-table-column :title="$t('tg-card.reconciliation.databaseBalance')" key="recorded" :width="160"><template #default="{record}">{{money(record.recordedBalanceAtomic)}}</template></a-table-column>
     <a-table-column :title="$t('tg-card.reconciliation.ledgerBalance')" key="ledger" :width="160"><template #default="{record}">{{money(record.ledgerBalanceAtomic)}}</template></a-table-column>
     <a-table-column :title="$t('tg-card.reconciliation.difference')" key="difference" :width="180"><template #default="{record}">{{money(record.differenceAtomic)}}</template></a-table-column>
     <a-table-column :title="$t('tg-card.reconciliation.foundAt')" key="created" :width="190"><template #default="{record}">{{when(record.createdAt)}}</template></a-table-column>
    </a-table>
    <a-empty v-if="!detailLoading&&anomalies.length===0" :description="$t('tg-card.reconciliation.noAnomalies')"/>
   </a-spin>
  </a-modal>
 </div>
</template>
