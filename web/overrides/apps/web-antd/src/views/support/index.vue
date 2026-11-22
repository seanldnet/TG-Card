<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { useWindowSize } from '@vueuse/core';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';
interface Ticket{id:string;ticketNo:string;orderId:string;orderNo:string;botId:string;telegramUserId:string;category:string;description:string;status:string;adminReply:string;assignedTo?:string;slaDueAt?:string;createdAt:string;updatedAt:string;resolvedAt?:string;aiStatus:string;aiReason?:string}
interface TicketMessage{id:string;ticketId:string;senderType:string;senderId:string;message:string;attachmentFileId?:string;attachmentName?:string;createdAt:string}
const loading=ref(false),saving=ref(false),tickets=ref<Ticket[]>([]),selected=ref<Ticket|null>(null),ticketModalOpen=ref(false),editStatus=ref('open');
const timeline=ref<TicketMessage[]>([]),replyText=ref(''),slaHours=ref(24),timelineLoading=ref(false),aiFilter=ref('all');
const filteredTickets=computed(()=>aiFilter.value==='all'?tickets.value:tickets.value.filter(x=>x.aiStatus===aiFilter.value));
const {width}=useWindowSize(),isMobile=computed(()=>width.value<768);
const openCount=computed(()=>tickets.value.filter(x=>['open','in_progress','waiting_user'].includes(x.status)).length);
const resolvedCount=computed(()=>tickets.value.filter(x=>x.status==='resolved').length);
function aiStatusLabel(v:string){const key='tg-card.support.ai.'+v;const value=$t(key);return value===key?$t('tg-card.support.ai.unknown'):value}
function aiStatusColor(v:string){return v==='ai_replied'?'success':v==='human_required'?'warning':v==='ai_error'?'error':'default'}
function statusLabel(v:string){const key='tg-card.support.status.'+v;const value=$t(key);return value===key?v:value}
function statusColor(v:string){if(v==='resolved')return 'success';if(v==='rejected'||v==='closed')return 'default';if(v==='waiting_user')return 'warning';return 'processing'}
function categoryLabel(v:string){const key='tg-card.support.category.'+v;const value=$t(key);return value===key?v:value}
function formatTime(v:string){const d=new Date(v);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
async function loadTickets(){loading.value=true;try{tickets.value=await requestClient.get<Ticket[]>('/support/tickets')}catch{message.error($t('tg-card.support.loadFailed'))}finally{loading.value=false}}
async function openEdit(item:Ticket){
 selected.value=item;replyText.value='';slaHours.value=24;
 editStatus.value=item.status;
 ticketModalOpen.value=true;timelineLoading.value=true;
 try{timeline.value=await requestClient.get<TicketMessage[]>('/support/tickets/'+item.id+'/messages')}
 catch{timeline.value=[];message.warning($t('tg-card.support.timelineLoadFailed'))}
 finally{timelineLoading.value=false}
}
async function assignTicket(){
 if(!selected.value)return;
 try{selected.value=await requestClient.put<Ticket>('/support/tickets/'+selected.value.id+'/assign',{slaHours:slaHours.value});message.success($t('tg-card.support.assigned'));await loadTickets()}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.support.assignFailed'))}
}
function refundTicket(item:Ticket){
 Modal.confirm({title:$t('tg-card.support.refundTitle'),content:$t('tg-card.support.refundDescription'),okType:'danger',okText:$t('tg-card.support.refundConfirm'),cancelText:$t('tg-card.support.cancel'),onOk:async()=>{
  try{const path='/api/support/tickets/'+item.id+'/refund';const result=await withAdminActionConfirmation({actionKey:'support.ticket.refund',method:'POST',path,run:(token)=>requestClient.post<any>('/support/tickets/'+item.id+'/refund',undefined,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success(result?.warning||$t('tg-card.support.refundSuccess'));await loadTickets()}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.support.refundFailed'))}
 }});
}
function resendOrder(item:Ticket){
 Modal.confirm({title:$t('tg-card.support.resendTitle'),content:$t('tg-card.support.resendDescription'),okText:$t('tg-card.support.resendConfirm'),cancelText:$t('tg-card.support.cancel'),onOk:async()=>{
  try{const path='/api/orders/'+item.orderId+'/retry-delivery';await withAdminActionConfirmation({actionKey:'order.delivery.retry',method:'POST',path,run:(token)=>requestClient.post('/orders/'+item.orderId+'/retry-delivery',undefined,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success($t('tg-card.support.resendSubmitted'))}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.support.resendFailed'))}
 }});
}
async function save(){
 if(!selected.value)return;
 saving.value=true;
 try{
  // Deliver and persist the reply before changing ticket status. If Telegram
  // rejects the message, the selected status must not be applied.
  if(replyText.value.trim()){
   await requestClient.post<any>('/support/tickets/'+selected.value.id+'/messages',{message:replyText.value.trim()});
   replyText.value='';
  }
  await requestClient.put('/support/tickets/'+selected.value.id,{status:editStatus.value,adminReply:''});
  message.success($t('tg-card.support.saved'));
  ticketModalOpen.value=false;await loadTickets();
 }catch(error:any){message.error(error?.response?.data?.message||error?.message||$t('tg-card.support.updateFailed'))}finally{saving.value=false}
}
onMounted(loadTickets);

</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-3 md:gap-4">
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.support.total')}}</div><div class="mt-2 text-2xl font-bold">{{tickets.length}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:ticket-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.support.pending')}}</div><div class="mt-2 text-2xl font-bold">{{openCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:clock-alert-outline" class="block text-2xl" /></span></div></a-card>
   <a-card :bordered="false" class="h-full"><div class="flex min-w-0 items-center justify-between gap-3"><div class="min-w-0"><div class="text-sm text-gray-500">{{$t('tg-card.support.resolved')}}</div><div class="mt-2 text-2xl font-bold">{{resolvedCount}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:check-circle-outline" class="block text-2xl" /></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3 px-1 py-1"><div class="min-w-0"><div class="text-base font-semibold leading-6">{{$t('tg-card.support.title')}}</div><div class="mt-1 text-xs leading-5 text-gray-500">{{$t('tg-card.support.subtitle')}}</div></div><div class="flex flex-wrap items-center gap-2"><a-select v-model:value="aiFilter" size="small" class="w-36" :options="[{label:$t('tg-card.support.allAI'),value:'all'},{label:$t('tg-card.support.ai.not_used'),value:'not_used'},{label:$t('tg-card.support.ai.ai_replied'),value:'ai_replied'},{label:$t('tg-card.support.ai.human_required'),value:'human_required'},{label:$t('tg-card.support.ai.ai_error'),value:'ai_error'}]"/><a-button size="small" class="mr-1" :loading="loading" @click="loadTickets">{{$t('tg-card.support.refresh')}}</a-button></div></div></template>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in filteredTickets" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0"><div class="truncate font-semibold">{{item.ticketNo}}</div><div class="mt-1 break-all text-xs text-gray-500">{{$t('tg-card.support.orderLabel')}}{{item.orderNo}}</div></div><div class="flex shrink-0 flex-col items-end gap-1"><a-tag :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag><a-tag :color="aiStatusColor(item.aiStatus)">{{aiStatusLabel(item.aiStatus)}}</a-tag></div></div>
     <div class="mt-3 text-sm font-medium">{{categoryLabel(item.category)}}</div><div class="mt-1 break-words text-sm text-gray-600">{{item.description}}</div>
     <div class="mt-3 flex items-center justify-between gap-2"><span class="text-xs text-gray-500">{{formatTime(item.createdAt)}}</span><a-space size="small"><a-button size="small" @click="resendOrder(item)">{{$t('tg-card.support.resend')}}</a-button><a-button size="small" danger @click="refundTicket(item)">{{$t('tg-card.support.refund')}}</a-button><a-button size="small" type="primary" @click="openEdit(item)">{{$t('tg-card.support.handle')}}</a-button></a-space></div>
    </a-card>
    <a-empty v-if="!loading&&filteredTickets.length===0" :description="$t('tg-card.support.empty')"/>
   </div>
   <a-table v-else :data-source="filteredTickets" :loading="loading" row-key="id" :pagination="{pageSize:10,showSizeChanger:true}" :scroll="{x:1200}">
    <a-table-column :title="$t('tg-card.support.ticketNo')" data-index="ticketNo" key="ticketNo" :width="190"/><a-table-column :title="$t('tg-card.support.orderNo')" data-index="orderNo" key="orderNo" :width="190"/><a-table-column :title="$t('tg-card.support.telegramUser')" data-index="telegramUserId" key="telegramUserId" :width="140"/>
    <a-table-column :title="$t('tg-card.support.categoryTitle')" key="category" :width="120"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column><a-table-column :title="$t('tg-card.support.description')" data-index="description" key="description" ellipsis/>
    <a-table-column :title="$t('tg-card.support.statusTitle')" key="status" :width="120"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column><a-table-column :title="$t('tg-card.support.aiHandling')" key="aiStatus" :width="130"><template #default="{record}"><a-tag :color="aiStatusColor(record.aiStatus)">{{aiStatusLabel(record.aiStatus)}}</a-tag><div v-if="record.aiReason" class="mt-1 text-xs text-gray-500">{{record.aiReason}}</div></template></a-table-column><a-table-column :title="$t('tg-card.support.submittedAt')" key="createdAt" :width="180"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.support.actions')" key="action" :width="210" fixed="right"><template #default="{record}"><a-space><a-button type="link" size="small" @click="resendOrder(record)">{{$t('tg-card.support.resendShort')}}</a-button><a-button type="link" danger size="small" @click="refundTicket(record)">{{$t('tg-card.support.refund')}}</a-button><a-button type="link" size="small" @click="openEdit(record)">{{$t('tg-card.support.handle')}}</a-button></a-space></template></a-table-column>
   </a-table>
  </a-card>
  <a-modal v-model:open="ticketModalOpen" :title="$t('tg-card.support.modalTitle')" :confirm-loading="saving" width="min(720px, 96vw)" :ok-text="$t('tg-card.support.save')" :cancel-text="$t('tg-card.support.cancel')" @ok="save">
   <div class="p-4 sm:p-6">
    <div v-if="selected" class="mb-4 break-words rounded-lg bg-gray-50 p-3 text-sm"><div><b>{{$t('tg-card.support.ticketLabel')}}</b>{{selected.ticketNo}}</div><div><b>{{$t('tg-card.support.orderLabel')}}</b>{{selected.orderNo}}</div><div class="mt-1"><b>{{$t('tg-card.support.issue')}}</b>{{selected.description}}</div><div class="mt-1"><b>{{$t('tg-card.support.aiLabel')}}</b><a-tag :color="aiStatusColor(selected.aiStatus)">{{aiStatusLabel(selected.aiStatus)}}</a-tag><span v-if="selected.aiReason" class="ml-1 text-xs text-gray-500">{{selected.aiReason}}</span></div></div>
    <div class="mb-4">
     <div class="mb-2 flex flex-wrap items-center justify-between gap-2"><b>{{$t('tg-card.support.timeline')}}</b><a-button size="small" :loading="timelineLoading" @click="selected&&openEdit(selected)">{{$t('tg-card.support.refreshTimeline')}}</a-button></div>
     <div class="max-h-64 space-y-2 overflow-y-auto rounded-lg border border-gray-100 p-3">
      <div v-if="timeline.length===0" class="py-4 text-center text-xs text-gray-400">{{$t('tg-card.support.noHistory')}}</div>
      <div v-for="entry in timeline" :key="entry.id" class="rounded-md bg-gray-50 p-3 text-sm">
       <div class="mb-1 flex justify-between gap-2 text-xs text-gray-500"><span>{{entry.senderType==='system'&&entry.senderId==='ai-assistant'?$t('tg-card.support.aiCustomerService'):entry.senderType==='admin'?$t('tg-card.support.admin'):entry.senderType==='system'?$t('tg-card.support.system'):$t('tg-card.support.user')}} · {{entry.senderId}}</span><span>{{formatTime(entry.createdAt)}}</span></div>
       <div v-if="entry.message" class="whitespace-pre-wrap break-words">{{entry.message}}</div>
       <div v-if="entry.attachmentFileId" class="mt-2 text-xs text-gray-500">{{entry.attachmentName?.toLowerCase().includes('图片')?$t('tg-card.support.image'):$t('tg-card.support.file')}} {{$t('tg-card.support.attachmentHint')}}</div>
      </div>
     </div>
    </div>
    <div class="mb-4"><b class="mb-2 block">{{$t('tg-card.support.assignment')}}</b><div class="flex flex-wrap items-center gap-2"><a-select v-model:value="slaHours" class="w-36" :options="[{label:$t('tg-card.support.hours4'),value:4},{label:$t('tg-card.support.hours8'),value:8},{label:$t('tg-card.support.hours24'),value:24},{label:$t('tg-card.support.hours48'),value:48},{label:$t('tg-card.support.hours72'),value:72}]"/><a-button size="small" @click="assignTicket">{{$t('tg-card.support.assignMe')}}</a-button></div><div v-if="selected?.assignedTo" class="mt-1 text-xs text-gray-500">{{$t('tg-card.support.assignee')}}{{selected.assignedTo}} · {{$t('tg-card.support.slaDeadline')}}{{selected.slaDueAt?formatTime(selected.slaDueAt):$t('tg-card.support.notSet')}}</div></div>
    <a-form-item :label="$t('tg-card.support.statusTitle')"><a-select v-model:value="editStatus" class="w-full" :options="[{label:$t('tg-card.support.status.open'),value:'open'},{label:$t('tg-card.support.status.in_progress'),value:'in_progress'},{label:$t('tg-card.support.status.waiting_user'),value:'waiting_user'},{label:$t('tg-card.support.status.resolved'),value:'resolved'},{label:$t('tg-card.support.status.rejected'),value:'rejected'},{label:$t('tg-card.support.status.closed'),value:'closed'}]"/></a-form-item>
    <a-form-item :label="$t('tg-card.support.replyLabel')"><a-textarea v-model:value="replyText" :rows="3" :maxlength="5000" show-count :placeholder="$t('tg-card.support.replyPlaceholder')"/></a-form-item>
   </div>
  </a-modal>
 </div>
</template>
