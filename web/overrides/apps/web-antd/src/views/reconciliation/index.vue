<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

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
function runLabel(value:string){return value.startsWith('manual-')?'手动检查 · '+value.slice(7):value}
function money(value:number){return (Number(value||0)/1_000_000).toFixed(6)+' USDT'}
function when(value:string){const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString()}
async function loadAnomalies(runKey:string, open=true){selectedRun.value=runKey;if(open)detailOpen.value=true;detailLoading.value=true;anomalies.value=[];try{anomalies.value=await requestClient.get<BalanceAnomaly[]>('/reconciliation/balance-anomalies',{params:{runKey}})}catch{message.error('余额异常明细加载失败')}finally{detailLoading.value=false}}
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
 }catch{message.error('对账数据加载失败')}finally{loading.value=false}
}
async function runCheck(){
 checking.value=true;
 try{
  const result=await requestClient.post<{run:BalanceRun;anomalies:BalanceAnomaly[]}>('/reconciliation/balance-check',{});
  message.success(result.run.mismatchUsers>0?'检查完成，发现余额差异，请核对明细':'检查完成，余额流水一致');
  await load();if(result.run.runKey)await loadAnomalies(result.run.runKey);
 }catch{message.error('余额一致性检查失败')}finally{checking.value=false}
}
const totals=computed(()=>reports.value.reduce((a,x)=>({orders:a.orders+x.orderCount,revenue:a.revenue+x.revenueAtomic,refund:a.refund+x.refundAtomic,recharge:a.recharge+x.rechargeAtomic,failed:a.failed+x.deliveryFailedCount,unmatched:a.unmatched+x.unmatchedTransactionCount,review:a.review+x.manualReviewTransactionCount}),{orders:0,revenue:0,refund:0,recharge:0,failed:0,unmatched:0,review:0}));
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-end justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">对账与余额校验</h1><p class="mb-0 mt-1 text-xs text-gray-500">每日 UTC 对账日报历史、链上异常统计及用户余额流水只读校验。</p></div>
   <div class="flex flex-wrap items-center gap-2"><a-date-picker v-model:value="month" picker="month" value-format="YYYY-MM" placeholder="选择月份" class="w-40" @change="load"/><a-button :loading="loading" @click="load">刷新</a-button><a-button type="primary" :loading="checking" @click="runCheck">立即执行余额校验</a-button></div>
  </div>
  <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
   <a-card :bordered="false"><div class="text-xs text-gray-500">本月订单数</div><div class="mt-2 text-2xl font-semibold">{{totals.orders}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">本月营收</div><div class="mt-2 text-xl font-semibold">{{money(totals.revenue)}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">本月退款</div><div class="mt-2 text-xl font-semibold">{{money(totals.refund)}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">本月充值</div><div class="mt-2 text-xl font-semibold">{{money(totals.recharge)}}</div></a-card>
  </div>
  <a-card :bordered="false" :loading="loading">
   <template #title><div class="text-base font-semibold">每日对账日历 · {{month}}</div></template>
   <a-alert class="mb-3" type="info" show-icon message="日报每日 UTC 00:05 后生成前一日快照；营收按 paid_at、订单数按 created_at、交付成功按 delivered_at、交付失败按失败事件时间统计。上游供应商余额取该 UTC 日内最近一次余额快照；历史日报为生成时快照。" />
   <a-table :data-source="reports" row-key="reportDate" :pagination="{pageSize:31,hideOnSinglePage:true}" :scroll="{x:1580}">
    <a-table-column title="日期（UTC）" data-index="reportDate" key="date" :width="130"/>
    <a-table-column title="营收" key="revenue" :width="150"><template #default="{record}">{{money(record.revenueAtomic)}}</template></a-table-column>
    <a-table-column title="订单数" data-index="orderCount" key="orders" :width="90"/>
    <a-table-column title="已交付" data-index="deliveredCount" key="delivered" :width="90"/>
    <a-table-column title="交付失败" data-index="deliveryFailedCount" key="failed" :width="100"/>
    <a-table-column title="退款金额" key="refund" :width="150"><template #default="{record}">{{money(record.refundAtomic)}}</template></a-table-column>
    <a-table-column title="充值总额" key="recharge" :width="150"><template #default="{record}">{{money(record.rechargeAtomic)}}</template></a-table-column>
    <a-table-column title="未匹配交易" data-index="unmatchedTransactionCount" key="unmatched" :width="110"/>
    <a-table-column title="需人工核对" data-index="manualReviewTransactionCount" key="review" :width="110"/>
    <a-table-column title="上游余额" key="supplierBalance" :width="160"><template #default="{record}">{{record.supplierBalance?record.supplierBalance+' '+record.supplierCurrency:'未获取'}}</template></a-table-column>
    <a-table-column title="上游余额检测时间" key="supplierBalanceCheckedAt" :width="190"><template #default="{record}">{{record.supplierBalanceCheckedAt?when(record.supplierBalanceCheckedAt):'—'}}</template></a-table-column>
    <a-table-column title="生成时间" key="generated" :width="190"><template #default="{record}">{{when(record.generatedAt)}}</template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&reports.length===0" description="本月暂无已生成的日报；日报在每日 UTC 00:05 后生成前一日数据"/>
  </a-card>
  <a-card :bordered="false" :loading="loading">
   <template #title><div class="flex flex-wrap items-center justify-between gap-2"><span class="text-base font-semibold">余额流水一致性检查</span><a-tag :color="anomalyCount>0?'error':'success'">{{anomalyCount>0?'最新检查发现异常':'最新检查无异常'}}</a-tag></div></template>
   <a-alert class="mb-3" type="warning" show-icon message="只读检测，不会自动修改用户余额或流水。账面余额按 credit 正数、debit 负数累计；站内退款当前是余额 credit，因此计入正向流水。" />
   <a-table :data-source="runs" row-key="runKey" :pagination="{pageSize:8,hideOnSinglePage:true}" :scroll="{x:800}">
    <a-table-column title="检查时间" key="time" :width="190"><template #default="{record}">{{when(record.runAt)}}</template></a-table-column>
    <a-table-column title="检查用户" data-index="checkedUsers" key="checked" :width="110"/>
    <a-table-column title="异常用户" data-index="mismatchUsers" key="mismatch" :width="110"/>
    <a-table-column title="差异绝对值合计" key="diff" :width="180"><template #default="{record}">{{money(record.totalDifferenceAtomic)}}</template></a-table-column>
    <a-table-column title="操作" key="action" :width="120"><template #default="{record}"><a-button size="small" @click="loadAnomalies(record.runKey)">查看差异明细</a-button></template></a-table-column>
   </a-table>

  </a-card>
  <a-modal v-model:open="detailOpen" title="余额差异明细" :footer="null" width="900px">
   <div class="mb-3 text-xs text-gray-500">检查批次：{{runLabel(selectedRun)}}</div>
   <a-spin :spinning="detailLoading">
    <a-table :data-source="anomalies" row-key="telegramUserId" :pagination="{pageSize:10,hideOnSinglePage:true}" :scroll="{x:850}">
     <a-table-column title="Telegram 用户 ID" data-index="telegramUserId" key="user" :width="180"/>
     <a-table-column title="数据库余额" key="recorded" :width="160"><template #default="{record}">{{money(record.recordedBalanceAtomic)}}</template></a-table-column>
     <a-table-column title="流水累计" key="ledger" :width="160"><template #default="{record}">{{money(record.ledgerBalanceAtomic)}}</template></a-table-column>
     <a-table-column title="差额（余额－流水）" key="difference" :width="180"><template #default="{record}">{{money(record.differenceAtomic)}}</template></a-table-column>
     <a-table-column title="发现时间" key="created" :width="190"><template #default="{record}">{{when(record.createdAt)}}</template></a-table-column>
    </a-table>
    <a-empty v-if="!detailLoading&&anomalies.length===0" description="本次检查没有余额差异"/>
   </a-spin>
  </a-modal>
 </div>
</template>
