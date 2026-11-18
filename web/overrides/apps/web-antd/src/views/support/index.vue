<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { useWindowSize } from '@vueuse/core';
import { requestClient } from '#/api/request';
interface Ticket{id:string;ticketNo:string;orderId:string;orderNo:string;botId:string;telegramUserId:string;category:string;description:string;status:string;adminReply:string;assignedTo?:string;slaDueAt?:string;createdAt:string;updatedAt:string;resolvedAt?:string}
interface TicketMessage{id:string;ticketId:string;senderType:string;senderId:string;message:string;attachmentFileId?:string;attachmentName?:string;createdAt:string}
const loading=ref(false),saving=ref(false),tickets=ref<Ticket[]>([]),selected=ref<Ticket|null>(null),ticketModalOpen=ref(false),editStatus=ref('open');
const timeline=ref<TicketMessage[]>([]),replyText=ref(''),slaHours=ref(24),timelineLoading=ref(false);
const {width}=useWindowSize(),isMobile=computed(()=>width.value<768);
const openCount=computed(()=>tickets.value.filter(x=>['open','in_progress','waiting_user'].includes(x.status)).length);
const resolvedCount=computed(()=>tickets.value.filter(x=>x.status==='resolved').length);
function statusLabel(v:string){return ({open:'待受理',in_progress:'处理中',waiting_user:'待用户补充',resolved:'已解决',rejected:'已拒绝',closed:'已关闭'} as Record<string,string>)[v]||v}
function statusColor(v:string){if(v==='resolved')return 'success';if(v==='rejected'||v==='closed')return 'default';if(v==='waiting_user')return 'warning';return 'processing'}
function categoryLabel(v:string){return ({not_received:'未收到货',invalid_key:'卡密无效',duplicate_payment:'重复付款',other:'其他问题'} as Record<string,string>)[v]||v}
function formatTime(v:string){const d=new Date(v);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
async function loadTickets(){loading.value=true;try{tickets.value=await requestClient.get<Ticket[]>('/support/tickets')}catch{message.error('售后工单加载失败')}finally{loading.value=false}}
async function openEdit(item:Ticket){
 selected.value=item;replyText.value='';slaHours.value=24;
 editStatus.value=item.status;
 ticketModalOpen.value=true;timelineLoading.value=true;
 try{timeline.value=await requestClient.get<TicketMessage[]>('/support/tickets/'+item.id+'/messages')}
 catch{timeline.value=[];message.warning('沟通记录暂时无法加载')}
 finally{timelineLoading.value=false}
}
async function assignTicket(){
 if(!selected.value)return;
 try{selected.value=await requestClient.put<Ticket>('/support/tickets/'+selected.value.id+'/assign',{slaHours:slaHours.value});message.success('工单已分配给当前管理员');await loadTickets()}
 catch(error:any){message.error(error?.response?.data?.message||'分配工单失败')}
}
function refundTicket(item:Ticket){
 Modal.confirm({title:'确认退款至站内余额',content:'将订单金额退还到用户站内余额，并撤销已交付卡密记录。该操作不可重复，也无法自动撤回外部链上转账。',okType:'danger',okText:'确认退款',cancelText:'取消',onOk:async()=>{
  try{const result=await requestClient.post<any>('/support/tickets/'+item.id+'/refund');message.success(result?.warning||'退款已入账并撤销卡密记录');await loadTickets()}
  catch(error:any){message.error(error?.response?.data?.message||'退款失败')}
 }});
}
function resendOrder(item:Ticket){
 Modal.confirm({title:'重新发送商品',content:'系统将尝试重新交付该工单关联订单的商品。',okText:'重新发送',cancelText:'取消',onOk:async()=>{
  try{await requestClient.post('/orders/'+item.orderId+'/retry-delivery');message.success('已提交重新交付任务')}
  catch(error:any){message.error(error?.response?.data?.message||'重新交付失败')}
 }});
}
async function save(){
 if(!selected.value)return;
 saving.value=true;
 try{
  await requestClient.put('/support/tickets/'+selected.value.id,{status:editStatus.value,adminReply:''});
  let notifyFailed=false;
  if(replyText.value.trim()){
   const response=await requestClient.post<any>('/support/tickets/'+selected.value.id+'/messages',{message:replyText.value.trim()});
   if(response?.notificationError)notifyFailed=true;
  }
  if(notifyFailed)message.warning('工单已保存，但 Telegram 通知发送失败');
  else message.success('工单状态和沟通记录已保存');
  ticketModalOpen.value=false;await loadTickets();
 }catch(error:any){message.error(error?.response?.data?.message||error?.message||'工单更新失败')}finally{saving.value=false}
}
onMounted(loadTickets);

</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-3 md:gap-4">
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">工单总数</div><div class="mt-2 text-2xl font-bold">{{tickets.length}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:ticket-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">待处理</div><div class="mt-2 text-2xl font-bold">{{openCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:clock-alert-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">已解决</div><div class="mt-2 text-2xl font-bold">{{resolvedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:check-circle-outline" class="block text-2xl" /></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3 px-1 py-1"><div class="min-w-0"><div class="text-base font-semibold leading-6">售后工单</div><div class="mt-1 text-xs leading-5 text-gray-500">工单文字在此查看；图片和文件凭证统一通过对应机器人转发给 Owner，请在机器人私聊中查看。</div></div><a-button size="small" class="mr-1" :loading="loading" @click="loadTickets">刷新工单</a-button></div></template>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in tickets" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0"><div class="truncate font-semibold">{{item.ticketNo}}</div><div class="mt-1 break-all text-xs text-gray-500">订单：{{item.orderNo}}</div></div><a-tag class="shrink-0" :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag></div>
     <div class="mt-3 text-sm font-medium">{{categoryLabel(item.category)}}</div><div class="mt-1 break-words text-sm text-gray-600">{{item.description}}</div>
     <div class="mt-3 flex items-center justify-between gap-2"><span class="text-xs text-gray-500">{{formatTime(item.createdAt)}}</span><a-space size="small"><a-button size="small" @click="resendOrder(item)">重发商品</a-button><a-button size="small" danger @click="refundTicket(item)">退款</a-button><a-button size="small" type="primary" @click="openEdit(item)">处理</a-button></a-space></div>
    </a-card>
    <a-empty v-if="!loading&&tickets.length===0" description="暂无售后工单"/>
   </div>
   <a-table v-else :data-source="tickets" :loading="loading" row-key="id" :pagination="{pageSize:10,showSizeChanger:true}" :scroll="{x:1200}">
    <a-table-column title="工单号" data-index="ticketNo" key="ticketNo" :width="190"/><a-table-column title="订单号" data-index="orderNo" key="orderNo" :width="190"/><a-table-column title="Telegram 用户" data-index="telegramUserId" key="telegramUserId" :width="140"/>
    <a-table-column title="问题类型" key="category" :width="120"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column><a-table-column title="问题描述" data-index="description" key="description" ellipsis/>
    <a-table-column title="状态" key="status" :width="120"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column><a-table-column title="提交时间" key="createdAt" :width="180"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
    <a-table-column title="操作" key="action" :width="210" fixed="right"><template #default="{record}"><a-space><a-button type="link" size="small" @click="resendOrder(record)">重发</a-button><a-button type="link" danger size="small" @click="refundTicket(record)">退款</a-button><a-button type="link" size="small" @click="openEdit(record)">处理</a-button></a-space></template></a-table-column>
   </a-table>
  </a-card>
  <a-modal v-model:open="ticketModalOpen" title="处理售后工单" :confirm-loading="saving" width="min(720px, 96vw)" ok-text="保存处理" cancel-text="取消" @ok="save">
   <div class="p-4 sm:p-6">
    <div v-if="selected" class="mb-4 break-words rounded-lg bg-gray-50 p-3 text-sm"><div><b>工单号：</b>{{selected.ticketNo}}</div><div><b>订单号：</b>{{selected.orderNo}}</div><div class="mt-1"><b>问题：</b>{{selected.description}}</div></div>
    <div class="mb-4">
     <div class="mb-2 flex flex-wrap items-center justify-between gap-2"><b>沟通时间线</b><a-button size="small" :loading="timelineLoading" @click="selected&&openEdit(selected)">刷新记录</a-button></div>
     <div class="max-h-64 space-y-2 overflow-y-auto rounded-lg border border-gray-100 p-3">
      <div v-if="timeline.length===0" class="py-4 text-center text-xs text-gray-400">暂无历史沟通记录</div>
      <div v-for="entry in timeline" :key="entry.id" class="rounded-md bg-gray-50 p-3 text-sm">
       <div class="mb-1 flex justify-between gap-2 text-xs text-gray-500"><span>{{entry.senderType==='admin'?'管理员':entry.senderType==='system'?'系统':'用户'}} · {{entry.senderId}}</span><span>{{formatTime(entry.createdAt)}}</span></div>
       <div v-if="entry.message" class="whitespace-pre-wrap break-words">{{entry.message}}</div>
       <div v-if="entry.attachmentFileId" class="mt-2 text-xs text-gray-500">{{entry.attachmentName?.toLowerCase().includes('图片')?'[图片]':'[文件]'}} 附件请通过机器人查看</div>
      </div>
     </div>
    </div>
    <div class="mb-4"><b class="mb-2 block">分配与 SLA</b><div class="flex flex-wrap items-center gap-2"><a-select v-model:value="slaHours" class="w-36" :options="[{label:'4 小时',value:4},{label:'8 小时',value:8},{label:'24 小时',value:24},{label:'48 小时',value:48},{label:'72 小时',value:72}]"/><a-button size="small" @click="assignTicket">分配给我并设置 SLA</a-button></div><div v-if="selected?.assignedTo" class="mt-1 text-xs text-gray-500">当前负责人：{{selected.assignedTo}} · SLA 截止：{{selected.slaDueAt?formatTime(selected.slaDueAt):'未设置'}}</div></div>
    <a-form-item label="工单状态"><a-select v-model:value="editStatus" class="w-full" :options="[{label:'待受理',value:'open'},{label:'处理中',value:'in_progress'},{label:'待用户补充',value:'waiting_user'},{label:'已解决',value:'resolved'},{label:'已拒绝',value:'rejected'},{label:'已关闭',value:'closed'}]"/></a-form-item>
    <a-form-item label="发送给用户的回复"><a-textarea v-model:value="replyText" :rows="3" :maxlength="5000" show-count placeholder="回复会记录在时间线，并通过原 Telegram Bot 私聊通知用户"/></a-form-item>
   </div>
  </a-modal>
 </div>
</template>
