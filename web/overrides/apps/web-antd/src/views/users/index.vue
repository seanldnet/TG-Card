<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';

interface TelegramBuyer {
  id:string;telegramUserId:string;username:string;firstName:string;lastName:string;
  lastSeenBotId:string;balanceAtomic:number;orderCount:number;totalSpentAtomic:number;
  createdAt:string;updatedAt:string;
}
const loading=ref(false),users=ref<TelegramBuyer[]>([]),keyword=ref('');
const modalOpen=ref(false),saving=ref(false),selected=ref<TelegramBuyer|null>(null);
const direction=ref<'credit'|'debit'>('credit'),amount=ref(''),note=ref('');
const filtered=computed(()=>{const q=keyword.value.trim().toLowerCase();return users.value.filter(u=>!q||[u.telegramUserId,u.username,u.firstName,u.lastName].join(' ').toLowerCase().includes(q));});
const totalSpent=computed(()=>users.value.reduce((sum,u)=>sum+u.totalSpentAtomic,0));
const totalBalance=computed(()=>users.value.reduce((sum,u)=>sum+u.balanceAtomic,0));
function usdt(value:number){return (value/1_000_000).toFixed(6)+' USDT';}
function displayName(u:TelegramBuyer){return [u.firstName,u.lastName].filter(Boolean).join(' ')|| (u.username?'@'+u.username:'Telegram 用户');}
function profileLink(u:TelegramBuyer){return u.username?'https://t.me/'+u.username:'tg://user?id='+u.telegramUserId;}
function formatTime(value:string){if(!value)return '-';const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString();}
function copy(value:string){navigator.clipboard?.writeText(value).then(()=>message.success('已复制')).catch(()=>message.error('复制失败'));}
async function load(){loading.value=true;try{users.value=await requestClient.get<TelegramBuyer[]>('/users');}catch{message.error('用户列表加载失败');}finally{loading.value=false;}}
function openBalance(u:TelegramBuyer,kind:'credit'|'debit'){selected.value=u;direction.value=kind;amount.value='';note.value='';modalOpen.value=true;}
async function submitBalance(){
 const value=Number(amount.value);
 if(!selected.value||!Number.isFinite(value)||value<=0||value>1_000_000_000){message.warning('请输入有效的 USDT 金额');return;}
 const atomic=Math.round(value*1_000_000);
 if(Math.abs(value*1_000_000-atomic)>0.000001){message.warning('金额最多支持 6 位小数');return;}
 saving.value=true;
 try{await requestClient.post('/users/'+selected.value.id+'/balance',{amountAtomic:atomic,direction:direction.value,note:note.value});message.success(direction.value==='credit'?'余额增加成功':'余额扣减成功');modalOpen.value=false;await load();}
 catch(error:any){message.error(error?.response?.data?.message||'余额调整失败');}
 finally{saving.value=false;}
}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
  <div><h1 class="m-0 text-xl font-semibold">用户管理</h1><p class="mb-0 mt-1 text-xs leading-5 text-gray-500">自动收集通过机器人互动的 Telegram 用户资料、订单消费及站内余额。</p></div>
  <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
   <a-card :bordered="false"><div class="flex items-center justify-between"><div><div class="text-sm text-gray-500">用户总数</div><div class="mt-2 text-2xl font-bold">{{users.length}}</div></div><span class="flex h-11 w-11 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:account-group-outline" class="text-2xl"/></span></div></a-card>
   <a-card :bordered="false"><div class="flex items-center justify-between"><div><div class="text-sm text-gray-500">用户站内余额</div><div class="mt-2 text-xl font-bold">{{usdt(totalBalance)}}</div></div><span class="flex h-11 w-11 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:wallet-outline" class="text-2xl"/></span></div></a-card>
   <a-card :bordered="false"><div class="flex items-center justify-between"><div><div class="text-sm text-gray-500">累计消费</div><div class="mt-2 text-xl font-bold">{{usdt(totalSpent)}}</div></div><span class="flex h-11 w-11 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:chart-line" class="text-2xl"/></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{padding:'12px 20px'}">
   <template #title><div class="flex flex-wrap items-center justify-between gap-3"><div class="font-semibold">Telegram 用户</div><a-button size="small" :loading="loading" @click="load">刷新用户</a-button></div></template>
   <a-input v-model:value="keyword" allow-clear class="mb-4 max-w-md" placeholder="搜索 Telegram ID、用户名或姓名"/>
   <a-table :data-source="filtered" :loading="loading" row-key="id" :scroll="{x:1000}" :pagination="{pageSize:20,showSizeChanger:true}">
    <a-table-column title="用户" key="user" :width="220"><template #default="{record}"><div class="min-w-0"><div class="font-medium">{{displayName(record)}}</div><div v-if="record.username" class="text-xs text-gray-500">@{{record.username}}</div><div class="text-xs text-gray-400">ID: {{record.telegramUserId}}</div></div></template></a-table-column>
    <a-table-column title="用户链接" key="link" :width="150"><template #default="{record}"><a v-if="record.username" :href="profileLink(record)" target="_blank" rel="noreferrer">打开 Telegram</a><a-button v-else type="link" size="small" @click="copy(profileLink(record))">复制用户链接</a-button></template></a-table-column>
    <a-table-column title="站内余额" key="balance" :width="150"><template #default="{record}">{{usdt(record.balanceAtomic)}}</template></a-table-column>
    <a-table-column title="累计消费" key="spent" :width="150"><template #default="{record}">{{usdt(record.totalSpentAtomic)}}</template></a-table-column>
    <a-table-column title="订单数" data-index="orderCount" key="orders" :width="90"/>
    <a-table-column title="最近互动" key="updated" :width="190"><template #default="{record}">{{formatTime(record.updatedAt)}}</template></a-table-column>
    <a-table-column title="余额操作" key="actions" :width="180" fixed="right"><template #default="{record}"><a-space><a-button size="small" type="link" @click="openBalance(record,'credit')">增加</a-button><a-button size="small" type="link" danger @click="openBalance(record,'debit')">扣减</a-button></a-space></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&users.length===0" description="还没有用户互动记录。用户向机器人发送消息或点击菜单后会自动收录。"/>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="direction==='credit'?'增加用户余额':'扣减用户余额'" :confirm-loading="saving" @ok="submitBalance">
   <template v-if="selected">
    <div class="mb-3 rounded-lg bg-gray-50 p-3 text-sm"><div>{{displayName(selected)}} <span v-if="selected.username" class="text-gray-500">@{{selected.username}}</span></div><div class="mt-1 text-gray-500">当前余额：{{usdt(selected.balanceAtomic)}}</div></div>
    <a-form layout="vertical"><a-form-item label="金额（USDT）"><a-input v-model:value="amount" inputmode="decimal" placeholder="例如 10.50"/></a-form-item><a-form-item label="备注"><a-textarea v-model:value="note" :rows="3" maxlength="500" placeholder="填写本次余额调整原因"/></a-form-item></a-form>
    <div class="text-xs text-gray-500">调整记录会写入余额流水，并保留操作管理员和时间。</div>
   </template>
  </a-modal>
 </div>
</template>
