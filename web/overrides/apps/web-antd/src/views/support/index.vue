<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { useVbenModal } from '@vben/common-ui';
import { useVbenForm } from '#/adapter/form';
import { requestClient } from '#/api/request';
interface Ticket{id:string;ticketNo:string;orderId:string;orderNo:string;botId:string;telegramUserId:string;category:string;description:string;status:string;adminReply:string;createdAt:string;updatedAt:string;resolvedAt?:string}
const loading=ref(false),saving=ref(false),tickets=ref<Ticket[]>([]),selected=ref<Ticket|null>(null);
const {width}=useWindowSize(),isMobile=computed(()=>width.value<768);
const openCount=computed(()=>tickets.value.filter(x=>['open','in_progress','waiting_user'].includes(x.status)).length);
const resolvedCount=computed(()=>tickets.value.filter(x=>x.status==='resolved').length);
const [TicketForm,ticketFormApi]=useVbenForm({
 commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',
 schema:[
  {component:'Select',fieldName:'status',label:'工单状态',componentProps:{options:[{label:'待受理',value:'open'},{label:'处理中',value:'in_progress'},{label:'待用户补充',value:'waiting_user'},{label:'已解决',value:'resolved'},{label:'已拒绝',value:'rejected'},{label:'已关闭',value:'closed'}]}},
  {component:'Textarea',fieldName:'adminReply',label:'管理员回复',componentProps:{rows:4,maxlength:2000,showCount:true,placeholder:'填写后会通过原 Telegram Bot 私聊发送给用户'}},
 ],
});
const [TicketModal,ticketModalApi]=useVbenModal({title:'处理售后工单',onConfirm:save});
function statusLabel(v:string){return ({open:'待受理',in_progress:'处理中',waiting_user:'待用户补充',resolved:'已解决',rejected:'已拒绝',closed:'已关闭'} as Record<string,string>)[v]||v}
function statusColor(v:string){if(v==='resolved')return 'success';if(v==='rejected'||v==='closed')return 'default';if(v==='waiting_user')return 'warning';return 'processing'}
function categoryLabel(v:string){return ({not_received:'未收到货',invalid_key:'卡密无效',duplicate_payment:'重复付款',other:'其他问题'} as Record<string,string>)[v]||v}
function formatTime(v:string){const d=new Date(v);return Number.isNaN(d.getTime())?'-':d.toLocaleString()}
async function loadTickets(){loading.value=true;try{tickets.value=await requestClient.get<Ticket[]>('/support/tickets')}catch{message.error('售后工单加载失败')}finally{loading.value=false}}
async function openEdit(item:Ticket){selected.value=item;await ticketFormApi.setValues({status:item.status,adminReply:item.adminReply||''});ticketModalApi.open()}
async function save(){
 if(!selected.value)return;
 const {valid}=await ticketFormApi.validate();if(!valid)return;
 const values=await ticketFormApi.getValues();saving.value=true;
 try{
  const result=await requestClient.put<{ticket:Ticket;notificationSent:boolean}>('/support/tickets/'+selected.value.id,{status:values.status,adminReply:values.adminReply||''});
  if(values.adminReply&&result.notificationSent)message.success('工单已更新，回复已发送给用户');
  else if(values.adminReply&&values.adminReply!==selected.value.adminReply)message.warning('工单已保存，但 Bot 未能通知用户');
  else message.success('工单已更新');
  ticketModalApi.close();await loadTickets();
 }catch{message.error('工单更新失败')}finally{saving.value=false}
}
onMounted(loadTickets);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
  <div class="tg-mobile-metric-grid md:grid md:grid-cols-3 md:gap-4">
   <a-card :bordered="false"><div class="text-sm text-gray-500">工单总数</div><div class="mt-2 text-2xl font-bold">{{tickets.length}}</div></a-card>
   <a-card :bordered="false"><div class="text-sm text-gray-500">待处理</div><div class="mt-2 text-2xl font-bold">{{openCount}}</div></a-card>
   <a-card :bordered="false"><div class="text-sm text-gray-500">已解决</div><div class="mt-2 text-2xl font-bold">{{resolvedCount}}</div></a-card>
  </div>
  <a-card :bordered="false">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3"><div class="min-w-0"><div class="text-lg font-semibold">售后工单</div><div class="mt-1 text-sm text-gray-500">回复会通过原 Telegram Bot 私聊发送给用户</div></div><a-button :loading="loading" @click="loadTickets">刷新工单</a-button></div></template>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in tickets" :key="item.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0"><div class="truncate font-semibold">{{item.ticketNo}}</div><div class="mt-1 break-all text-xs text-gray-500">订单：{{item.orderNo}}</div></div><a-tag class="shrink-0" :color="statusColor(item.status)">{{statusLabel(item.status)}}</a-tag></div>
     <div class="mt-3 text-sm font-medium">{{categoryLabel(item.category)}}</div><div class="mt-1 break-words text-sm text-gray-600">{{item.description}}</div>
     <div class="mt-3 flex items-center justify-between gap-2"><span class="text-xs text-gray-500">{{formatTime(item.createdAt)}}</span><a-button size="small" type="primary" @click="openEdit(item)">处理工单</a-button></div>
    </a-card>
    <a-empty v-if="!loading&&tickets.length===0" description="暂无售后工单"/>
   </div>
   <a-table v-else :data-source="tickets" :loading="loading" row-key="id" :pagination="{pageSize:10,showSizeChanger:true}" :scroll="{x:1200}">
    <a-table-column title="工单号" data-index="ticketNo" key="ticketNo" :width="190"/><a-table-column title="订单号" data-index="orderNo" key="orderNo" :width="190"/><a-table-column title="Telegram 用户" data-index="telegramUserId" key="telegramUserId" :width="140"/>
    <a-table-column title="问题类型" key="category" :width="120"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column><a-table-column title="问题描述" data-index="description" key="description" ellipsis/>
    <a-table-column title="状态" key="status" :width="120"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column><a-table-column title="提交时间" key="createdAt" :width="180"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
    <a-table-column title="操作" key="action" :width="100" fixed="right"><template #default="{record}"><a-button type="link" size="small" @click="openEdit(record)">处理工单</a-button></template></a-table-column>
   </a-table>
  </a-card>
  <TicketModal class="w-full max-w-xl" :confirm-loading="saving">
   <div class="p-4 sm:p-6">
    <div v-if="selected" class="mb-4 break-words rounded-lg bg-gray-50 p-3 text-sm"><div><b>工单号：</b>{{selected.ticketNo}}</div><div><b>订单号：</b>{{selected.orderNo}}</div><div class="mt-1"><b>问题：</b>{{selected.description}}</div></div>
    <TicketForm/>
   </div>
  </TicketModal>
 </div>
</template>
