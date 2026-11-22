<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface SystemException {
  id:string; sourceKey:string; category:string; severity:string; entityType:string; entityId:string;
  title:string; detail:string; status:string; resolutionNote:string; resolvedBy:string;
  createdAt:string; updatedAt:string; resolvedAt:string;
}
const items=ref<SystemException[]>([]);
const loading=ref(false), saving=ref(false), modalOpen=ref(false);
const status=ref('open'), category=ref(''), severity=ref('');
const fromDate=ref<string>(), toDate=ref<string>();
const openCount=ref(0);
const selected=ref<SystemException|null>(null);
const nextStatus=ref('resolved'), note=ref('');
const categoryOptions=[
  {value:'delivery_uncertain',label:$t('tg-card.systemExceptions.category.delivery_uncertain')},
  {value:'refund_conflict',label:$t('tg-card.systemExceptions.category.refund_conflict')},
  {value:'upstream_sync',label:$t('tg-card.systemExceptions.category.upstream_sync')},
  {value:'cap_verification',label:$t('tg-card.systemExceptions.category.cap_verification')},
  {value:'cap_configuration',label:$t('tg-card.systemExceptions.category.cap_configuration')},
  {value:'procurement_failed',label:$t('tg-card.systemExceptions.category.procurement_failed')},
  {value:'procurement_uncertain',label:$t('tg-card.systemExceptions.category.procurement_uncertain')},
  {value:'supplier_balance',label:$t('tg-card.systemExceptions.category.supplier_balance')},
  {value:'refund_ledger',label:$t('tg-card.systemExceptions.category.refund_ledger')},
];
const categoryLabel=(v:string)=>{const key='tg-card.systemExceptions.category.'+v;const value=$t(key);return value===key?$t('tg-card.systemExceptions.other'):value};
const entityTypeLabel=(v:string)=>{const key='tg-card.systemExceptions.entity.'+v;const value=$t(key);return value===key?v:value};
const sourceLabel=(v:string)=>v.startsWith('manual-')?$t('tg-card.systemExceptions.sourceManual')+v.slice(7):v;
const severityLabel=(v:string)=>{const key='tg-card.systemExceptions.severity.'+v;const value=$t(key);return value===key?v:value};
const severityColor=(v:string)=>({critical:'red',error:'orange',warning:'gold',info:'blue'} as Record<string,string>)[v]||'default';
const statusLabel=(v:string)=>{const key='tg-card.systemExceptions.status.'+v;const value=$t(key);return value===key?v:value};
const dateToISO=(value?:string, end=false)=>value?new Date(value+'T'+(end?'23:59:59.999':'00:00:00.000')+'Z').toISOString():'';
const query=computed(()=>({
  status:status.value, category:category.value, severity:severity.value, limit:300,
  ...(fromDate.value?{from:dateToISO(fromDate.value)}:{}),
  ...(toDate.value?{to:dateToISO(toDate.value,true)}:{}),
}));
async function load(){
  loading.value=true;
  try{
    const [list,count]=await Promise.all([
      requestClient.get<SystemException[]>('/system/exceptions',{params:query.value}),
      requestClient.get<{openCount:number}>('/system/exceptions/count'),
    ]);
    items.value=list||[];openCount.value=count?.openCount||0;
  }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.systemExceptions.loadFailed'));}
  finally{loading.value=false;}
}
function openAction(item:SystemException,target:string){
  selected.value=item;nextStatus.value=target;note.value='';
  modalOpen.value=true;
}
async function save(){
  if(!selected.value)return;
  if(nextStatus.value==='resolved'&&!note.value.trim()){message.warning($t('tg-card.systemExceptions.resolutionRequired'));return;}
  saving.value=true;
  try{
    await requestClient.put('/system/exceptions/'+encodeURIComponent(selected.value.id),{status:nextStatus.value,note:note.value.trim()});
    message.success($t('tg-card.systemExceptions.updated'));modalOpen.value=false;await load();
  }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.systemExceptions.updateFailed'));}
  finally{saving.value=false;}
}
function resetFilters(){status.value='open';category.value='';severity.value='';fromDate.value=undefined;toDate.value=undefined;load();}
onMounted(load);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">{{$t('tg-card.systemExceptions.title')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.systemExceptions.subtitle')}}</p></div>
   <div class="flex items-center gap-2"><a-badge :count="openCount" :overflow-count="999"><a-tag color="error">{{$t('tg-card.systemExceptions.pending')}}</a-tag></a-badge><a-button size="small" :loading="loading" @click="load">{{$t('tg-card.systemExceptions.refresh')}}</a-button></div>
  </div>
  <a-card :bordered="false">
   <a-form layout="vertical"><div class="grid grid-cols-1 gap-3 md:grid-cols-5">
    <a-form-item :label="$t('tg-card.systemExceptions.filters.status')" class="mb-0"><a-select v-model:value="status"><a-select-option value="">{{$t('tg-card.systemExceptions.filters.all')}}</a-select-option><a-select-option value="open">{{$t('tg-card.systemExceptions.status.open')}}</a-select-option><a-select-option value="processing">{{$t('tg-card.systemExceptions.processingButton')}}</a-select-option><a-select-option value="resolved">{{$t('tg-card.systemExceptions.status.resolved')}}</a-select-option><a-select-option value="ignored">{{$t('tg-card.systemExceptions.status.ignored')}}</a-select-option></a-select></a-form-item>
    <a-form-item :label="$t('tg-card.systemExceptions.filters.category')" class="mb-0"><a-select v-model:value="category" allow-clear :placeholder="$t('tg-card.systemExceptions.filters.allTypes')"><a-select-option v-for="option in categoryOptions" :key="option.value" :value="option.value">{{option.label}}</a-select-option></a-select></a-form-item>
    <a-form-item :label="$t('tg-card.systemExceptions.filters.severity')" class="mb-0"><a-select v-model:value="severity" allow-clear :placeholder="$t('tg-card.systemExceptions.filters.allLevels')"><a-select-option value="critical">{{$t('tg-card.systemExceptions.severity.critical')}}</a-select-option><a-select-option value="error">{{$t('tg-card.systemExceptions.severity.error')}}</a-select-option><a-select-option value="warning">{{$t('tg-card.systemExceptions.severity.warning')}}</a-select-option><a-select-option value="info">{{$t('tg-card.systemExceptions.severity.info')}}</a-select-option></a-select></a-form-item>
    <a-form-item :label="$t('tg-card.systemExceptions.filters.from')" class="mb-0"><a-date-picker v-model:value="fromDate" value-format="YYYY-MM-DD" class="w-full" /></a-form-item>
    <a-form-item :label="$t('tg-card.systemExceptions.filters.to')" class="mb-0"><a-date-picker v-model:value="toDate" value-format="YYYY-MM-DD" class="w-full" /></a-form-item>
   </div></a-form>
   <div class="mt-3 flex flex-wrap justify-end gap-2"><a-button size="small" @click="resetFilters">{{$t('tg-card.systemExceptions.reset')}}</a-button><a-button size="small" type="primary" :loading="loading" @click="load">{{$t('tg-card.systemExceptions.filter')}}</a-button></div>
  </a-card>
  <a-card :bordered="false" :title="$t('tg-card.systemExceptions.records')" :loading="loading">
   <a-table :data-source="items" row-key="id" :pagination="{pageSize:15,showSizeChanger:true}" :scroll="{x:1200}">
    <a-table-column :title="$t('tg-card.systemExceptions.severityTitle')" key="severity" :width="90"><template #default="{record}"><a-tag :color="severityColor(record.severity)">{{severityLabel(record.severity)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.systemExceptions.categoryTitle')" key="category" :width="150"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.systemExceptions.issue')" data-index="title" :width="230"/>
    <a-table-column :title="$t('tg-card.systemExceptions.entity')" key="entity" :width="190"><template #default="{record}">{{entityTypeLabel(record.entityType)}} · {{record.entityId}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.systemExceptions.detail')" data-index="detail" :width="340" :ellipsis="true"/>
    <a-table-column :title="$t('tg-card.systemExceptions.filters.status')" key="status" :width="100"><template #default="{record}"><a-tag>{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.systemExceptions.occurred')" data-index="createdAt" :width="190"/>
    <a-table-column :title="$t('tg-card.systemExceptions.action')" key="action" :fixed="'right'" :width="190"><template #default="{record}"><div class="flex gap-1"><a-button size="small" @click="openAction(record,'processing')">{{$t('tg-card.systemExceptions.processingButton')}}</a-button><a-button size="small" type="primary" :disabled="record.status==='resolved'" @click="openAction(record,'resolved')">{{$t('tg-card.systemExceptions.resolve')}}</a-button></div></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&!items.length" :description="$t('tg-card.systemExceptions.empty')"/>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="$t('tg-card.systemExceptions.modalTitle')" :confirm-loading="saving" @ok="save">
   <template v-if="selected">
    <div class="mb-3 rounded-lg bg-gray-50 p-3"><div class="font-medium">{{selected.title}}</div><div class="mt-2 whitespace-pre-wrap break-words text-xs text-gray-600">{{selected.detail}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.systemExceptions.source')}}{{sourceLabel(selected.sourceKey)}} · {{$t('tg-card.systemExceptions.occurred')}}{{selected.createdAt}}</div></div>
    <a-form-item :label="$t('tg-card.systemExceptions.processingStatus')"><a-select v-model:value="nextStatus"><a-select-option value="processing">{{$t('tg-card.systemExceptions.processingButton')}}</a-select-option><a-select-option value="resolved">{{$t('tg-card.systemExceptions.status.resolved')}}</a-select-option><a-select-option value="ignored">{{$t('tg-card.systemExceptions.ignore')}}</a-select-option></a-select></a-form-item>
    <a-form-item :label="$t('tg-card.systemExceptions.note')"><a-textarea v-model:value="note" :rows="4" :maxlength="2000" show-count :placeholder="$t('tg-card.systemExceptions.notePlaceholder')"/></a-form-item>
   </template>
  </a-modal>
 </div>
</template>
