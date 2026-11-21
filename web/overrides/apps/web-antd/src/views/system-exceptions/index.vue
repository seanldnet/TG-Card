<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

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
  {value:'delivery_uncertain',label:'交付结果不确定'},
  {value:'refund_conflict',label:'退款冲突'},
  {value:'upstream_sync',label:'上游同步失败'},
  {value:'cap_verification',label:'Cap 验证失败'},
  {value:'cap_configuration',label:'Cap 配置异常'},
  {value:'procurement_failed',label:'上游采购失败'},
  {value:'procurement_uncertain',label:'采购受理不确定'},
  {value:'supplier_balance',label:'供应商资金差异'},
  {value:'refund_ledger',label:'退款账本异常'},
];
const categoryLabel=(v:string)=>categoryOptions.find(x=>x.value===v)?.label||v||'其他';
const entityTypeLabel=(v:string)=>({order:'订单',admin:'管理员',supplier:'供应商',payment:'支付',wallet:'钱包',telegram_user:'Telegram 用户',admin_login:'管理员登录'} as Record<string,string>)[v]||v;
const sourceLabel=(v:string)=>v.startsWith('manual-')?'人工记录 · '+v.slice(7):v;
const severityLabel=(v:string)=>({critical:'严重',error:'错误',warning:'警告',info:'提示'} as Record<string,string>)[v]||v;
const severityColor=(v:string)=>({critical:'red',error:'orange',warning:'gold',info:'blue'} as Record<string,string>)[v]||'default';
const statusLabel=(v:string)=>({open:'待处理',processing:'处理中',resolved:'已解决',ignored:'已忽略'} as Record<string,string>)[v]||v;
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
  }catch(error:any){message.error(error?.response?.data?.message||'读取全局异常失败');}
  finally{loading.value=false;}
}
function openAction(item:SystemException,target:string){
  selected.value=item;nextStatus.value=target;note.value='';
  modalOpen.value=true;
}
async function save(){
  if(!selected.value)return;
  if(nextStatus.value==='resolved'&&!note.value.trim()){message.warning('标记解决前请填写处理结论');return;}
  saving.value=true;
  try{
    await requestClient.put('/system/exceptions/'+encodeURIComponent(selected.value.id),{status:nextStatus.value,note:note.value.trim()});
    message.success('异常状态已更新，操作审计已记录');modalOpen.value=false;await load();
  }catch(error:any){message.error(error?.response?.data?.message||'更新异常状态失败');}
  finally{saving.value=false;}
}
function resetFilters(){status.value='open';category.value='';severity.value='';fromDate.value=undefined;toDate.value=undefined;load();}
onMounted(load);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">全局异常中心</h1><p class="mb-0 mt-1 text-xs text-gray-500">统一查看交付不确定、退款冲突、上游同步、Cap 验证及资金异常；处理动作写入管理员审计日志。</p></div>
   <div class="flex items-center gap-2"><a-badge :count="openCount" :overflow-count="999"><a-tag color="error">待处理异常</a-tag></a-badge><a-button :loading="loading" @click="load">刷新</a-button></div>
  </div>
  <a-card :bordered="false">
   <div class="grid grid-cols-1 gap-3 md:grid-cols-5">
    <a-form-item label="状态" class="mb-0"><a-select v-model:value="status"><a-select-option value="">全部</a-select-option><a-select-option value="open">待处理</a-select-option><a-select-option value="processing">处理中</a-select-option><a-select-option value="resolved">已解决</a-select-option><a-select-option value="ignored">已忽略</a-select-option></a-select></a-form-item>
    <a-form-item label="异常类型" class="mb-0"><a-select v-model:value="category" allow-clear placeholder="全部类型"><a-select-option v-for="option in categoryOptions" :key="option.value" :value="option.value">{{option.label}}</a-select-option></a-select></a-form-item>
    <a-form-item label="严重程度" class="mb-0"><a-select v-model:value="severity" allow-clear placeholder="全部级别"><a-select-option value="critical">严重</a-select-option><a-select-option value="error">错误</a-select-option><a-select-option value="warning">警告</a-select-option><a-select-option value="info">提示</a-select-option></a-select></a-form-item>
    <a-form-item label="开始日期" class="mb-0"><a-date-picker v-model:value="fromDate" value-format="YYYY-MM-DD" class="w-full" /></a-form-item>
    <a-form-item label="结束日期" class="mb-0"><a-date-picker v-model:value="toDate" value-format="YYYY-MM-DD" class="w-full" /></a-form-item>
   </div>
   <div class="mt-2 flex justify-end gap-2"><a-button @click="resetFilters">重置</a-button><a-button type="primary" :loading="loading" @click="load">筛选</a-button></div>
  </a-card>
  <a-card :bordered="false" title="异常记录" :loading="loading">
   <a-table :data-source="items" row-key="id" :pagination="{pageSize:15,showSizeChanger:true}" :scroll="{x:1200}">
    <a-table-column title="级别" key="severity" :width="90"><template #default="{record}"><a-tag :color="severityColor(record.severity)">{{severityLabel(record.severity)}}</a-tag></template></a-table-column>
    <a-table-column title="类型" key="category" :width="150"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column>
    <a-table-column title="异常事项" data-index="title" :width="230"/>
    <a-table-column title="关联对象" key="entity" :width="190"><template #default="{record}">{{entityTypeLabel(record.entityType)}} · {{record.entityId}}</template></a-table-column>
    <a-table-column title="错误详情" data-index="detail" :width="340" :ellipsis="true"/>
    <a-table-column title="状态" key="status" :width="100"><template #default="{record}"><a-tag>{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="发生时间" data-index="createdAt" :width="190"/>
    <a-table-column title="处理操作" key="action" :fixed="'right'" :width="190"><template #default="{record}"><div class="flex gap-1"><a-button size="small" @click="openAction(record,'processing')">处理中</a-button><a-button size="small" type="primary" :disabled="record.status==='resolved'" @click="openAction(record,'resolved')">标记解决</a-button></div></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&!items.length" description="当前筛选条件下没有异常记录"/>
  </a-card>
  <a-modal v-model:open="modalOpen" title="处理异常记录" :confirm-loading="saving" @ok="save">
   <template v-if="selected">
    <div class="mb-3 rounded-lg bg-gray-50 p-3"><div class="font-medium">{{selected.title}}</div><div class="mt-2 whitespace-pre-wrap break-words text-xs text-gray-600">{{selected.detail}}</div><div class="mt-2 text-xs text-gray-400">来源：{{sourceLabel(selected.sourceKey)}} · 发生时间：{{selected.createdAt}}</div></div>
    <a-form-item label="处理状态"><a-select v-model:value="nextStatus"><a-select-option value="processing">处理中</a-select-option><a-select-option value="resolved">已解决</a-select-option><a-select-option value="ignored">忽略</a-select-option></a-select></a-form-item>
    <a-form-item label="处理说明"><a-textarea v-model:value="note" :rows="4" :maxlength="2000" show-count placeholder="填写排查结论、处理措施或忽略原因"/></a-form-item>
   </template>
  </a-modal>
 </div>
</template>
