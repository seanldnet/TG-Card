<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface Bot { id:string; name:string; username:string; status:number }
interface Task { id:string; botId:string; title:string; messageText:string; status:string; scheduledAt:string; totalCount:number; sentCount:number; failedCount:number; createdAt:string }
const loading=ref(false),sending=ref(false),bots=ref<Bot[]>([]),tasks=ref<Task[]>([]);
const form=reactive({botId:'',title:'',messageText:'',scheduledAt:''});
const selectedBot=computed(()=>bots.value.find(x=>x.id===form.botId));
function statusText(v:string){const key='tg-card.broadcasts.status.'+v;const value=$t(key);return value===key?v:value}
function statusColor(v:string){return v==='completed'?'success':v==='failed'?'error':v==='completed_with_errors'?'warning':'processing'}
function formatTime(v:string){const d=new Date(v);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
async function load(){loading.value=true;try{const [b,t]=await Promise.all([requestClient.get<Bot[]>('/bots'),requestClient.get<Task[]>('/broadcasts')]);bots.value=b;tasks.value=t;if(!form.botId&&b.length)form.botId=b.find(x=>x.status===1)?.id||b[0].id}catch{message.error($t('tg-card.broadcasts.loadFailed'))}finally{loading.value=false}}
async function submit(){
 if(!form.botId){message.warning($t('tg-card.broadcasts.chooseBot'));return}
 if(!form.messageText.trim()){message.warning($t('tg-card.broadcasts.messageRequired'));return}
 Modal.confirm({title:$t('tg-card.broadcasts.confirmTitle'),content:$t('tg-card.broadcasts.confirmDescription'),okText:$t('tg-card.broadcasts.confirmCreate'),cancelText:$t('tg-card.broadcasts.cancel'),onOk:async()=>{
  sending.value=true;
  try{await requestClient.post('/broadcasts',{botId:form.botId,title:form.title.trim(),messageText:form.messageText.trim(),scheduledAt:form.scheduledAt||''});message.success($t('tg-card.broadcasts.created'));form.title='';form.messageText='';form.scheduledAt='';await load()}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.broadcasts.createFailed'))}
  finally{sending.value=false}
 }});
}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3"><div><h1 class="m-0 text-xl font-semibold">{{$t('tg-card.broadcasts.title')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.broadcasts.subtitle')}}</p></div><a-button :loading="loading" @click="load">{{$t('tg-card.broadcasts.refresh')}}</a-button></div>
  <a-card :bordered="false" :title="$t('tg-card.broadcasts.createTask')">
   <a-form layout="vertical">
    <a-form-item :label="$t('tg-card.broadcasts.bot')" required><a-select v-model:value="form.botId" :options="bots.map(b=>({label:b.name+' (@'+b.username+')',value:b.id,disabled:b.status!==1}))" :placeholder="$t('tg-card.broadcasts.selectBot')"/></a-form-item>
    <a-form-item :label="$t('tg-card.broadcasts.taskName')"><a-input v-model:value="form.title" maxlength="100" :placeholder="$t('tg-card.broadcasts.taskPlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.broadcasts.message')" required><a-textarea v-model:value="form.messageText" :rows="6" :maxlength="4000" show-count :placeholder="$t('tg-card.broadcasts.messagePlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.broadcasts.scheduled')"><a-date-picker v-model:value="form.scheduledAt" class="w-full sm:max-w-sm" show-time value-format="YYYY-MM-DDTHH:mm:ssZ" :placeholder="$t('tg-card.broadcasts.selectTime')"/></a-form-item>
    <div class="flex flex-wrap items-center gap-3"><a-button type="primary" :loading="sending" @click="submit">{{$t('tg-card.broadcasts.createTask')}}</a-button><span class="text-xs text-gray-500">{{$t('tg-card.broadcasts.rateLimit')}}</span></div>
   </a-form>
  </a-card>
  <a-card :bordered="false" :title="$t('tg-card.broadcasts.records')">
   <a-table :data-source="tasks" :loading="loading" row-key="id" :pagination="{pageSize:10}" :scroll="{x:900}">
    <a-table-column :title="$t('tg-card.broadcasts.taskName')" data-index="title" key="title"><template #default="{record}">{{record.title||$t('tg-card.broadcasts.unnamed')}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.broadcasts.content')" data-index="messageText" key="messageText" ellipsis/>
    <a-table-column :title="$t('tg-card.broadcasts.scheduledAt')" key="scheduledAt"><template #default="{record}">{{formatTime(record.scheduledAt)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.broadcasts.statusTitle')" key="status"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusText(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.broadcasts.progress')" key="progress"><template #default="{record}">{{record.sentCount}} / {{record.totalCount}} {{$t('tg-card.broadcasts.successCount')}} · {{record.failedCount}} {{$t('tg-card.broadcasts.failedCount')}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.broadcasts.createdAt')" key="createdAt"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
   </a-table>
  </a-card>
 </div>
</template>
