<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface Ticket { id:string; ticketNo:string; orderId:string; orderNo:string; botId:string; telegramUserId:string; category:string; description:string; status:string; adminReply:string; createdAt:string; updatedAt:string; resolvedAt?:string }
const loading=ref(false); const saving=ref(false); const modalOpen=ref(false); const tickets=ref<Ticket[]>([]); const selected=ref<Ticket|null>(null);
const form=reactive({status:'in_progress',adminReply:''});
const openCount=computed(()=>tickets.value.filter(x=>['open','in_progress','waiting_user'].includes(x.status)).length);
const resolvedCount=computed(()=>tickets.value.filter(x=>x.status==='resolved').length);
function statusLabel(value:string){const labels:Record<string,string>={open:'待受理',in_progress:'处理中',waiting_user:'待用户补充',resolved:'已解决',rejected:'已拒绝',closed:'已关闭'};return labels[value]||value}
function statusColor(value:string){if(value==='resolved')return 'success';if(value==='rejected'||value==='closed')return 'default';if(value==='waiting_user')return 'warning';return 'processing'}
function categoryLabel(value:string){const labels:Record<string,string>={not_received:'未收到货',invalid_key:'卡密无效',duplicate_payment:'重复付款',other:'其他问题'};return labels[value]||value}
function formatTime(value:string){if(!value)return '-';const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString()}
async function loadTickets(){loading.value=true;try{tickets.value=await requestClient.get<Ticket[]>('/support/tickets')}catch{message.error('售后工单加载失败')}finally{loading.value=false}}
function openEdit(item:Ticket){selected.value=item;Object.assign(form,{status:item.status,adminReply:item.adminReply||''});modalOpen.value=true}
async function save(){if(!selected.value)return;saving.value=true;try{const result=await requestClient.put<{ticket:Ticket;notificationSent:boolean}>('/support/tickets/'+selected.value.id,{status:form.status,adminReply:form.adminReply});if(form.adminReply.trim()&&result.notificationSent){message.success('工单已更新，回复已发送给用户')}else if(form.adminReply.trim()&&form.adminReply!==selected.value.adminReply){message.warning('工单已保存，但 Bot 未能通知用户，请检查 Bot 运行状态')}else{message.success('工单已更新')}modalOpen.value=false;await loadTickets()}catch{message.error('工单更新失败')}finally{saving.value=false}}
onMounted(loadTickets);
</script>

<template>
  <div class="p-5">
    <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-3">
      <a-card :bordered="false"><div class="text-sm text-gray-500">工单总数（最近 500 条）</div><div class="mt-2 text-2xl font-semibold">{{ tickets.length }}</div></a-card>
      <a-card :bordered="false"><div class="text-sm text-gray-500">待处理</div><div class="mt-2 text-2xl font-semibold">{{ openCount }}</div></a-card>
      <a-card :bordered="false"><div class="text-sm text-gray-500">已解决</div><div class="mt-2 text-2xl font-semibold">{{ resolvedCount }}</div></a-card>
    </div>
    <a-card :bordered="false">
      <template #title><div class="flex flex-wrap items-center justify-between gap-3"><div><div class="text-lg font-semibold">售后工单</div><div class="mt-1 text-sm text-gray-500">按订单追踪售后问题，回复会通过原 Bot 私聊发送给用户</div></div><a-button :loading="loading" @click="loadTickets">刷新工单</a-button></div></template>
      <a-table :data-source="tickets" :loading="loading" row-key="id" :pagination="{pageSize:10,showSizeChanger:true}" :scroll="{x:1200}">
        <a-table-column title="工单号" data-index="ticketNo" key="ticketNo" :width="190" />
        <a-table-column title="订单号" data-index="orderNo" key="orderNo" :width="190" />
        <a-table-column title="Telegram 用户" data-index="telegramUserId" key="telegramUserId" :width="140" />
        <a-table-column title="问题类型" key="category" :width="120"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column>
        <a-table-column title="问题描述" data-index="description" key="description" ellipsis />
        <a-table-column title="状态" key="status" :width="120"><template #default="{record}"><a-tag :color="statusColor(record.status)">{{statusLabel(record.status)}}</a-tag></template></a-table-column>
        <a-table-column title="提交时间" key="createdAt" :width="180"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
        <a-table-column title="操作" key="action" :width="100" fixed="right"><template #default="{record}"><a-button type="link" size="small" @click="openEdit(record)">处理工单</a-button></template></a-table-column>
        <template #emptyText><a-empty description="暂无售后工单" /></template>
      </a-table>
    </a-card>
    <a-modal v-model:open="modalOpen" title="处理售后工单" :confirm-loading="saving" ok-text="保存并通知" cancel-text="取消" @ok="save">
      <template v-if="selected">
        <div class="mb-3 rounded bg-gray-50 p-3 text-sm"><div><b>工单号：</b>{{selected.ticketNo}}</div><div><b>订单号：</b>{{selected.orderNo}}</div><div><b>问题：</b>{{selected.description}}</div></div>
        <a-form layout="vertical"><a-form-item label="工单状态"><a-select v-model:value="form.status"><a-select-option value="open">待受理</a-select-option><a-select-option value="in_progress">处理中</a-select-option><a-select-option value="waiting_user">待用户补充</a-select-option><a-select-option value="resolved">已解决</a-select-option><a-select-option value="rejected">已拒绝</a-select-option><a-select-option value="closed">已关闭</a-select-option></a-select></a-form-item><a-form-item label="管理员回复"><a-textarea v-model:value="form.adminReply" :rows="4" :maxlength="2000" show-count placeholder="填写后会通过原 Telegram Bot 私聊发送给用户" /></a-form-item></a-form>
      </template>
    </a-modal>
  </div>
</template>
