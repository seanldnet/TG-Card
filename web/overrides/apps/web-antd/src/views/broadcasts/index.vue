<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface Bot { id:string; name:string; username:string; status:number }
interface Task { id:string; botId:string; title:string; messageText:string; status:string; scheduledAt:string; totalCount:number; sentCount:number; failedCount:number; createdAt:string }
const loading=ref(false),sending=ref(false),bots=ref<Bot[]>([]),tasks=ref<Task[]>([]);
const form=reactive({botId:'',title:'',messageText:'',scheduledAt:''});
const selectedBot=computed(()=>bots.value.find(x=>x.id===form.botId));
function statusText(v:string){return ({queued:'等待发送',sending:'发送中',completed:'已完成',completed_with_errors:'部分失败',failed:'发送失败'} as Record<string,string>)[v]||v}
function statusColor(v:string){return v==='completed'?'success':v==='failed'?'error':v==='completed_with_errors'?'warning':'processing'}
function formatTime(v:string){const d=new Date(v);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
async function load(){loading.value=true;try{const [b,t]=await Promise.all([requestClient.get<Bot[]>('/bots'),requestClient.get<Task[]>('/broadcasts')]);bots.value=b;tasks.value=t;if(!form.botId&&b.length)form.botId=b.find(x=>x.status===1)?.id||b[0].id}catch{message.error('群发数据加载失败')}finally{loading.value=false}}
async function submit(){
 if(!form.botId){message.warning('请先选择机器人');return}
 if(!form.messageText.trim()){message.warning('请输入群发内容');return}
 Modal.confirm({title:'确认创建群发任务',content:'系统将向该机器人已联系过且未退订的用户发送消息。请确认文案和发送对象。',okText:'确认创建',cancelText:'取消',onOk:async()=>{
  sending.value=true;
  try{await requestClient.post('/broadcasts',{botId:form.botId,title:form.title.trim(),messageText:form.messageText.trim(),scheduledAt:form.scheduledAt||''});message.success('群发任务已创建，将按计划自动发送');form.title='';form.messageText='';form.scheduledAt='';await load()}
  catch(error:any){message.error(error?.response?.data?.message||'创建群发任务失败')}
  finally{sending.value=false}
 }});
}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3"><div><h1 class="m-0 text-xl font-semibold">群发通知</h1><p class="mb-0 mt-1 text-xs text-gray-500">支持立即发送或定时发送，自动排除已退订用户并记录每位收件人的发送结果</p></div><a-button :loading="loading" @click="load">刷新任务</a-button></div>
  <a-card :bordered="false" title="创建群发任务">
   <a-form layout="vertical">
    <a-form-item label="发送机器人" required><a-select v-model:value="form.botId" :options="bots.map(b=>({label:b.name+' (@'+b.username+')',value:b.id,disabled:b.status!==1}))" placeholder="选择已启用的机器人"/></a-form-item>
    <a-form-item label="任务名称"><a-input v-model:value="form.title" maxlength="100" placeholder="例如：新品上架通知"/></a-form-item>
    <a-form-item label="消息内容" required><a-textarea v-model:value="form.messageText" :rows="6" :maxlength="4000" show-count placeholder="输入要发送给用户的通知内容"/></a-form-item>
    <a-form-item label="计划发送时间（留空则立即进入发送队列）"><a-date-picker v-model:value="form.scheduledAt" class="w-full sm:max-w-sm" show-time value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="选择发送时间"/></a-form-item>
    <div class="flex flex-wrap items-center gap-3"><a-button type="primary" :loading="sending" @click="submit">创建群发任务</a-button><span class="text-xs text-gray-500">发送频率受控，遇到 Telegram 限流会自动等待并重试</span></div>
   </a-form>
  </a-card>
  <a-card :bordered="false" title="发送记录">
   <a-table :data-source="tasks" :loading="loading" row-key="id" :pagination="{pageSize:10}" :scroll="{x:900}">
    <a-table-column title="任务名称" data-index="title" key="title"><template #default="{record}">{{record.title||'未命名群发'}}</template></a-table-column>
    <a-table-column title="发送内容" data-index="messageText" key="messageText" ellipsis/>
    <a-table-column title="计划时间" key="scheduledAt"><template #default="{record}">{{formatTime(record.scheduledAt)}}</template></a-table-column>
    <a-table-column title="发送状态" key="status"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusText(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="发送进度" key="progress"><template #default="{record}">{{record.sentCount}} / {{record.totalCount}} 成功 · {{record.failedCount}} 失败</template></a-table-column>
    <a-table-column title="创建时间" key="createdAt"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
   </a-table>
  </a-card>
 </div>
</template>
