<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface FinanceException {id:string;sourceKey:string;category:string;severity:string;entityType:string;entityId:string;title:string;detail:string;status:string;resolutionNote:string;resolvedBy:string;createdAt:string;updatedAt:string;resolvedAt:string}
interface UncertainOrder {localOrderId:string;localOrderNo:string;externalOrderId:string;externalOrderNo:string;status:string;lastError:string;createdAt:string;ageingSeconds:number;waitSeconds:number}
interface FundCheck {localOrderId:string;localOrderNo:string;externalOrderId:string;currency:string;balanceBefore:string;balanceAfter:string;expectedDebit:string;observedDebit:string;difference:string;status:string;detail:string;checkedAt:string}
const exceptions=ref<FinanceException[]>([]),uncertain=ref<UncertainOrder[]>([]),fundChecks=ref<FundCheck[]>([]);
const openCount=ref(0),loading=ref(false),checking=ref(false),statusFilter=ref('open');
const selected=ref<FinanceException|null>(null),note=ref(''),actionStatus=ref('resolved'),saving=ref(false),modalOpen=ref(false);
const uncertainSelected=ref<UncertainOrder|null>(null),resolution=ref('accepted'),externalOrderId=ref(''),externalOrderNo=ref(''),resolutionNote=ref(''),resolutionOpen=ref(false),resolving=ref(false);
const categoryLabel=(v:string)=>{const key='tg-card.financeExceptions.category.'+v;const value=$t(key);return value===key?v:value};
const severityColor=(v:string)=>v==='critical'?'red':v==='error'?'orange':'gold';
const severityLabel=(v:string)=>{const key='tg-card.financeExceptions.severity.'+v;const value=$t(key);return value===key?v:value};
const entityTypeLabel=(v:string)=>{const key='tg-card.financeExceptions.entity.'+v;const value=$t(key);return value===key?v:value};
const fundStatusLabel=(v:string)=>{const key='tg-card.financeExceptions.fundStatus.'+v;const value=$t(key);return value===key?v:value};
const statusLabel=(v:string)=>{const key='tg-card.financeExceptions.status.'+v;const value=$t(key);return value===key?v:value};
const ageing=(n:number)=>{const h=Math.floor(n/3600),d=Math.floor(h/24),m=Math.floor((n%3600)/60);return d>0?$t('tg-card.financeExceptions.ageingDayHours',{days:d,hours:h%24}):h>0?$t('tg-card.financeExceptions.ageingHoursMinutes',{hours:h,minutes:m}):$t('tg-card.financeExceptions.ageingMinutes',{minutes:Math.floor(n/60)});};
const openBadge=computed(()=>openCount.value>0?openCount.value:0);
async function load(){
 loading.value=true;
 try{
  const [list,count,unc,funds]=await Promise.all([
   requestClient.get<FinanceException[]>('/finance/exceptions',{params:{status:statusFilter.value,limit:200}}),
   requestClient.get<{openCount:number}>('/finance/exceptions/count'),
   requestClient.get<UncertainOrder[]>('/upstream/procurements/uncertain',{params:{limit:200}}),
   requestClient.get<FundCheck[]>('/upstream/fund-checks',{params:{limit:200}}),
  ]);
  exceptions.value=list||[];openCount.value=count?.openCount||0;uncertain.value=unc||[];fundChecks.value=funds||[];
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.financeExceptions.loadFailed'));}
 finally{loading.value=false;}
}
function openException(item:FinanceException,status='resolved'){selected.value=item;note.value='';actionStatus.value=status;modalOpen.value=true;}
async function saveException(){
 if(!selected.value)return;
 if(actionStatus.value==='resolved'&&!note.value.trim()){message.warning($t('tg-card.financeExceptions.resolutionRequired'));return;}
 saving.value=true;
 try{await requestClient.put('/finance/exceptions/'+selected.value.id,{status:actionStatus.value,note:note.value.trim()});message.success($t('tg-card.financeExceptions.updated'));modalOpen.value=false;await load();}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.financeExceptions.updateFailed'));}
 finally{saving.value=false;}
}
function openUncertain(item:UncertainOrder,choice:string){uncertainSelected.value=item;resolution.value=choice;externalOrderId.value=item.externalOrderId||'';externalOrderNo.value=item.externalOrderNo||'';resolutionNote.value='';resolutionOpen.value=true;}
async function resolveUncertain(){
 if(!uncertainSelected.value)return;
 if(!resolutionNote.value.trim()){message.warning($t('tg-card.financeExceptions.verificationRecord'));return;}
 if(resolution.value==='accepted'&&!externalOrderId.value.trim()){message.warning($t('tg-card.financeExceptions.supplierOrderIdPlaceholder'));return;}
 resolving.value=true;
 try{await requestClient.post('/upstream/procurements/'+uncertainSelected.value.localOrderId+'/resolve',{resolution:resolution.value,externalOrderId:externalOrderId.value.trim(),externalOrderNo:externalOrderNo.value.trim(),note:resolutionNote.value.trim()});message.success(resolution.value==='accepted'?$t('tg-card.upstream.messages.supplierAccepted'):$t('tg-card.upstream.messages.supplierNotAccepted'));resolutionOpen.value=false;await load();}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.resolve'));}
 finally{resolving.value=false;}
}
async function runRefundCheck(){
 checking.value=true;
 try{const result=await requestClient.post<{refundLedgerAnomalies:number;autoCorrection:boolean}>('/finance/refund-ledger/check',{});message.success($t('tg-card.financeExceptions.refundCheck',{count:result.refundLedgerAnomalies,autoCorrection:result.autoCorrection?$t('tg-card.financeExceptions.autoCorrection.executed'):$t('tg-card.financeExceptions.autoCorrection.notExecuted')}));await load();}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.financeExceptions.updateFailed'));}
 finally{checking.value=false;}
}
onMounted(load);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">{{$t('tg-card.financeExceptions.title')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.financeExceptions.subtitle')}}</p></div>
   <div class="flex items-center gap-2"><a-badge :count="openBadge" :overflow-count="999"><a-tag color="error">{{$t('tg-card.financeExceptions.openCount')}}</a-tag></a-badge><a-button size="small" :loading="checking" @click="runRefundCheck">{{$t('tg-card.financeExceptions.runRefundCheck')}}</a-button><a-button size="small" :loading="loading" @click="load">{{$t('tg-card.financeExceptions.refresh')}}</a-button></div>
  </div>
  <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.financeExceptions.openProcessing')}}</div><div class="mt-2 text-2xl font-semibold">{{openCount}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.financeExceptions.uncertainOrders')}}</div><div class="mt-2 text-2xl font-semibold">{{uncertain.length}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.financeExceptions.supplierBalance')}}</div><div class="mt-2 text-2xl font-semibold">{{fundChecks.filter(x=>x.status==='mismatch').length}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">{{$t('tg-card.financeExceptions.refundLedger')}}</div><div class="mt-2 text-2xl font-semibold">{{exceptions.filter(x=>x.category==='refund_ledger').length}}</div></a-card>
  </div>
  <a-card :bordered="false" :title="$t('tg-card.financeExceptions.exceptionList')" :loading="loading">
   <template #extra><a-select v-model:value="statusFilter" class="w-32" @change="load"><a-select-option value="open">{{$t('tg-card.financeExceptions.status.open')}}</a-select-option><a-select-option value="processing">{{$t('tg-card.financeExceptions.processing')}}</a-select-option><a-select-option value="resolved">{{$t('tg-card.financeExceptions.status.resolved')}}</a-select-option><a-select-option value="ignored">{{$t('tg-card.financeExceptions.status.ignored')}}</a-select-option></a-select></template>
   <a-table :data-source="exceptions" row-key="id" :pagination="{pageSize:10}" :scroll="{x:1100}">
    <a-table-column :title="$t('tg-card.financeExceptions.severityTitle')" key="severity" :width="90"><template #default="{record}"><a-tag :color="severityColor(record.severity)">{{severityLabel(record.severity)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.financeExceptions.categoryTitle')" key="category" :width="140"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.financeExceptions.issue')" data-index="title" :width="220"/>
    <a-table-column :title="$t('tg-card.financeExceptions.entity')" key="entity" :width="180"><template #default="{record}">{{entityTypeLabel(record.entityType)}} · {{record.entityId}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.financeExceptions.detail')" data-index="detail" :ellipsis="true" :width="320"/>
    <a-table-column :title="$t('tg-card.financeExceptions.statusTitle')" key="status" :width="110"><template #default="{record}"><a-tag>{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.financeExceptions.occurred')" data-index="createdAt" :width="190"/>
    <a-table-column :title="$t('tg-card.financeExceptions.handle')" key="actions" :fixed="'right'" :width="170"><template #default="{record}"><div class="flex gap-1"><a-button size="small" @click="openException(record,'processing')">{{$t('tg-card.financeExceptions.processing')}}</a-button><a-button size="small" type="primary" @click="openException(record,'resolved')">{{$t('tg-card.financeExceptions.complete')}}</a-button></div></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&!exceptions.length" :description="$t('tg-card.financeExceptions.empty')"/>
  </a-card>
  <a-card :bordered="false" :title="$t('tg-card.financeExceptions.uncertainList')" :loading="loading">
   <a-alert class="mb-3" type="warning" show-icon :message="$t('tg-card.financeExceptions.uncertainNotice')"/>
   <a-table :data-source="uncertain" row-key="localOrderId" :pagination="{pageSize:8}" :scroll="{x:1100}">
    <a-table-column :title="$t('tg-card.financeExceptions.localOrder')" data-index="localOrderNo" :width="190"/>
    <a-table-column :title="$t('tg-card.financeExceptions.externalOrderId')" data-index="externalOrderId" :width="130"/>
    <a-table-column :title="$t('tg-card.financeExceptions.ageing')" key="age" :width="130"><template #default="{record}">{{ageing(record.ageingSeconds)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.financeExceptions.reason')" data-index="lastError" :width="320"/>
    <a-table-column :title="$t('tg-card.financeExceptions.actions')" :fixed="'right'" :width="210"><template #default="{record}"><div class="flex gap-1"><a-button size="small" type="primary" @click="openUncertain(record,'accepted')">{{$t('tg-card.financeExceptions.supplierAccepted')}}</a-button><a-button size="small" danger @click="openUncertain(record,'not_accepted')">{{$t('tg-card.financeExceptions.supplierNotAccepted')}}</a-button></div></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&!uncertain.length" :description="$t('tg-card.financeExceptions.empty')"/>
  </a-card>
  <a-card :bordered="false" :title="$t('tg-card.financeExceptions.fundReconciliation')" :loading="loading">
   <a-alert class="mb-3" type="info" show-icon :message="$t('tg-card.financeExceptions.fundNotice')"/>
   <a-table :data-source="fundChecks" row-key="localOrderId" :pagination="{pageSize:8}" :scroll="{x:1250}">
    <a-table-column :title="$t('tg-card.financeExceptions.localOrder')" data-index="localOrderNo" :width="190"/>
    <a-table-column :title="$t('tg-card.financeExceptions.externalOrderId')" data-index="externalOrderId" :width="130"/>
    <a-table-column :title="$t('tg-card.financeExceptions.currency')" data-index="currency" :width="80"/>
    <a-table-column :title="$t('tg-card.financeExceptions.purchaseAmount')" data-index="expectedDebit" :width="120"/>
    <a-table-column :title="$t('tg-card.financeExceptions.balanceChange')" data-index="observedDebit" :width="120"/>
    <a-table-column :title="$t('tg-card.financeExceptions.difference')" data-index="difference" :width="110"/>
    <a-table-column :title="$t('tg-card.financeExceptions.result')" key="status" :width="120"><template #default="{record}"><a-tag :color="record.status==='matched'?'success':record.status==='mismatch'?'error':'warning'">{{fundStatusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.financeExceptions.detail')" data-index="detail" :width="300"/>
    <a-table-column :title="$t('tg-card.financeExceptions.checkedAt')" data-index="checkedAt" :width="190"/>
   </a-table>
   <a-empty v-if="!loading&&!fundChecks.length" :description="$t('tg-card.financeExceptions.fundEmpty')"/>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="$t('tg-card.financeExceptions.modalTitle')" :confirm-loading="saving" @ok="saveException">
   <div v-if="selected" class="mb-3"><div class="font-medium">{{selected.title}}</div><div class="mt-1 text-xs text-gray-500">{{selected.detail}}</div></div>
   <a-form-item :label="$t('tg-card.financeExceptions.statusField')"><a-select v-model:value="actionStatus"><a-select-option value="processing">{{$t('tg-card.financeExceptions.processing')}}</a-select-option><a-select-option value="resolved">{{$t('tg-card.financeExceptions.status.resolved')}}</a-select-option><a-select-option value="ignored">{{$t('tg-card.financeExceptions.status.ignored')}}</a-select-option></a-select></a-form-item>
   <a-form-item :label="$t('tg-card.financeExceptions.note')"><a-textarea v-model:value="note" :rows="3" :maxlength="500" show-count :placeholder="$t('tg-card.financeExceptions.notePlaceholder')"/></a-form-item>
  </a-modal>
  <a-modal v-model:open="resolutionOpen" :title="$t('tg-card.financeExceptions.resolutionModalTitle')" :confirm-loading="resolving" @ok="resolveUncertain">
   <div v-if="uncertainSelected" class="mb-3 text-sm">{{$t('tg-card.financeExceptions.localOrderLabel')}}{{uncertainSelected.localOrderNo}} · {{$t('tg-card.financeExceptions.waiting')}}{{ageing(uncertainSelected.ageingSeconds)}}</div>
   <a-form-item :label="$t('tg-card.financeExceptions.verificationResult')"><a-radio-group v-model:value="resolution"><a-radio value="accepted">{{$t('tg-card.financeExceptions.supplierAccepted')}}</a-radio><a-radio value="not_accepted">{{$t('tg-card.financeExceptions.supplierNotAccepted')}}</a-radio></a-radio-group></a-form-item>
   <a-form-item v-if="resolution==='accepted'" :label="$t('tg-card.financeExceptions.externalOrderId')"><a-input v-model:value="externalOrderId" :placeholder="$t('tg-card.financeExceptions.supplierOrderIdPlaceholder')"/></a-form-item>
   <a-form-item v-if="resolution==='accepted'" :label="$t('tg-card.financeExceptions.supplierOrderNo')"><a-input v-model:value="externalOrderNo" :placeholder="$t('tg-card.financeExceptions.supplierOrderNoPlaceholder')"/></a-form-item>
   <a-form-item :label="$t('tg-card.financeExceptions.verificationRecord')"><a-textarea v-model:value="resolutionNote" :rows="3" :maxlength="500" :placeholder="$t('tg-card.financeExceptions.verificationPlaceholder')"/></a-form-item>
  </a-modal>
 </div>
</template>
