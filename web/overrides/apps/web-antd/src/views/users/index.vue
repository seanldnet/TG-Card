<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';

interface TelegramBuyer {
  id:string;telegramUserId:string;username:string;firstName:string;lastName:string;
  lastSeenBotId:string;balanceAtomic:number;orderCount:number;totalSpentAtomic:number;
  createdAt:string;updatedAt:string;
}
const loading=ref(false),users=ref<TelegramBuyer[]>([]),keyword=ref('');
const modalOpen=ref(false),saving=ref(false),selected=ref<TelegramBuyer|null>(null);
const ledgerOpen=ref(false),ledgerLoading=ref(false),ledger=ref<any[]>([]);
const direction=ref<'credit'|'debit'>('credit'),amount=ref(''),note=ref('');
const filtered=computed(()=>{const q=keyword.value.trim().toLowerCase();return users.value.filter(u=>!q||[u.telegramUserId,u.username,u.firstName,u.lastName].join(' ').toLowerCase().includes(q));});
const totalSpent=computed(()=>users.value.reduce((sum,u)=>sum+u.totalSpentAtomic,0));
const totalBalance=computed(()=>users.value.reduce((sum,u)=>sum+u.balanceAtomic,0));
function usdt(value:number){return (value/1_000_000).toFixed(6)+' USDT';}
function displayName(u:TelegramBuyer){return [u.firstName,u.lastName].filter(Boolean).join(' ')|| (u.username?'@'+u.username:$t('tg-card.users.telegramUsers'));}
function profileLink(u:TelegramBuyer){return u.username?'https://t.me/'+u.username:'tg://user?id='+u.telegramUserId;}
function formatTime(value:string){if(!value)return '-';const d=new Date(value);return Number.isNaN(d.getTime())?value:d.toLocaleString();}
function copy(value:string){navigator.clipboard?.writeText(value).then(()=>message.success($t('tg-card.users.copied'))).catch(()=>message.error($t('tg-card.users.copyFailed')));}
async function load(){loading.value=true;try{users.value=await requestClient.get<TelegramBuyer[]>('/users');}catch{message.error($t('tg-card.users.loadFailed'));}finally{loading.value=false;}}
function openBalance(u:TelegramBuyer,kind:'credit'|'debit'){selected.value=u;direction.value=kind;amount.value='';note.value='';modalOpen.value=true;}
async function openLedger(u:TelegramBuyer){selected.value=u;ledgerOpen.value=true;ledgerLoading.value=true;try{ledger.value=await requestClient.get<any[]>('/users/'+u.id+'/wallet');}catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.users.ledgerFailed'));ledger.value=[];}finally{ledgerLoading.value=false;}}
async function submitBalance(){
 const value=Number(amount.value);
 if(!selected.value||!Number.isFinite(value)||value<=0||value>1_000_000_000){message.warning($t('tg-card.users.amountInvalid'));return;}
 const atomic=Math.round(value*1_000_000);
 if(Math.abs(value*1_000_000-atomic)>0.000001){message.warning($t('tg-card.users.precision'));return;}
 saving.value=true;
 try{const path='/api/users/'+selected.value.id+'/balance';const payload={amountAtomic:atomic,direction:direction.value,note:note.value};await withAdminActionConfirmation({actionKey:'wallet.balance.adjust',method:'POST',path,payload,run:(token)=>requestClient.post('/users/'+selected.value!.id+'/balance',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success(direction.value==='credit'?$t('tg-card.users.creditSuccess'):$t('tg-card.users.debitSuccess'));modalOpen.value=false;await load();}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.users.adjustFailed'));}
 finally{saving.value=false;}
}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div><h1 class="m-0 text-xl font-semibold">{{$t('tg-card.users.title')}}</h1><p class="mb-0 mt-1 text-xs leading-5 text-gray-500">{{$t('tg-card.users.subtitle')}}</p></div>
  <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
   <a-card :bordered="false"><div class="flex items-center justify-between"><div><div class="text-sm text-gray-500">{{$t('tg-card.users.total')}}</div><div class="mt-2 text-2xl font-bold">{{users.length}}</div></div><span class="flex h-11 w-11 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:account-group-outline" class="text-2xl"/></span></div></a-card>
   <a-card :bordered="false"><div class="flex items-center justify-between"><div><div class="text-sm text-gray-500">{{$t('tg-card.users.balance')}}</div><div class="mt-2 text-xl font-bold">{{usdt(totalBalance)}}</div></div><span class="flex h-11 w-11 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:wallet-outline" class="text-2xl"/></span></div></a-card>
   <a-card :bordered="false"><div class="flex items-center justify-between"><div><div class="text-sm text-gray-500">{{$t('tg-card.users.spent')}}</div><div class="mt-2 text-xl font-bold">{{usdt(totalSpent)}}</div></div><span class="flex h-11 w-11 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:chart-line" class="text-2xl"/></span></div></a-card>
  </div>
  <a-card :bordered="false" :head-style="{padding:'12px 20px'}">
   <template #title><div class="flex flex-wrap items-center justify-between gap-3"><div class="font-semibold">{{$t('tg-card.users.telegramUsers')}}</div><a-button size="small" :loading="loading" @click="load">{{$t('tg-card.users.refresh')}}</a-button></div></template>
   <a-input v-model:value="keyword" allow-clear class="mb-4 max-w-md" :placeholder="$t('tg-card.users.search')"/>
   <a-table :data-source="filtered" :loading="loading" row-key="id" :scroll="{x:1000}" :pagination="{pageSize:20,showSizeChanger:true}">
    <a-table-column :title="$t('tg-card.users.user')" key="user" :width="220"><template #default="{record}"><div class="min-w-0"><div class="font-medium">{{displayName(record)}}</div><div v-if="record.username" class="text-xs text-gray-500">@{{record.username}}</div><div class="text-xs text-gray-400">ID: {{record.telegramUserId}}</div></div></template></a-table-column>
    <a-table-column :title="$t('tg-card.users.userLink')" key="link" :width="150"><template #default="{record}"><a v-if="record.username" :href="profileLink(record)" target="_blank" rel="noreferrer">{{$t('tg-card.users.openTelegram')}}</a><a-button v-else type="link" size="small" @click="copy(profileLink(record))">{{$t('tg-card.users.copyLink')}}</a-button></template></a-table-column>
    <a-table-column :title="$t('tg-card.users.balance')" key="balance" :width="150"><template #default="{record}">{{usdt(record.balanceAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.users.spent')" key="spent" :width="150"><template #default="{record}">{{usdt(record.totalSpentAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.users.orders')" data-index="orderCount" key="orders" :width="90"/>
    <a-table-column :title="$t('tg-card.users.recentActivity')" key="updated" :width="190"><template #default="{record}">{{formatTime(record.updatedAt)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.users.balanceActions')" key="actions" :width="180" fixed="right"><template #default="{record}"><a-space><a-button size="small" type="link" @click="openLedger(record)">{{$t('tg-card.users.ledger')}}</a-button><a-button size="small" type="link" @click="openBalance(record,'credit')">{{$t('tg-card.users.credit')}}</a-button><a-button size="small" type="link" danger @click="openBalance(record,'debit')">{{$t('tg-card.users.debit')}}</a-button></a-space></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&users.length===0" :description="$t('tg-card.users.empty')"/>
  </a-card>
  <a-modal v-model:open="ledgerOpen" :title="$t('tg-card.users.ledgerTitle')" :footer="null" width="min(720px, 94vw)">
   <a-spin :spinning="ledgerLoading"><a-table :data-source="ledger" row-key="createdAt" size="small" :pagination="{pageSize:10}">
    <a-table-column :title="$t('tg-card.users.type')" key="direction" :width="90"><template #default="{record}"><a-tag :color="record.direction==='credit'?'success':'warning'">{{record.direction==='credit'?$t('tg-card.users.income'):$t('tg-card.users.expense')}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.users.amount')" key="amount" :width="150"><template #default="{record}">{{usdt(record.amountAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.users.balanceAfter')" key="balance" :width="150"><template #default="{record}">{{usdt(record.balanceAfterAtomic)}}</template></a-table-column>
    <a-table-column :title="$t('tg-card.users.note')" data-index="note" key="note"/>
    <a-table-column :title="$t('tg-card.users.time')" key="createdAt" :width="180"><template #default="{record}">{{formatTime(record.createdAt)}}</template></a-table-column>
   </a-table><a-empty v-if="!ledgerLoading&&ledger.length===0" :description="$t('tg-card.users.noLedger')"/></a-spin>
  </a-modal>
  <a-modal v-model:open="modalOpen" :title="direction==='credit'?$t('tg-card.users.creditTitle'):$t('tg-card.users.debitTitle')" :confirm-loading="saving" @ok="submitBalance">
   <template v-if="selected">
    <div class="mb-3 rounded-lg bg-gray-50 p-3 text-sm"><div>{{displayName(selected)}} <span v-if="selected.username" class="text-gray-500">@{{selected.username}}</span></div><div class="mt-1 text-gray-500">{{$t('tg-card.users.currentBalance')}}{{usdt(selected.balanceAtomic)}}</div></div>
    <a-form layout="vertical"><a-form-item :label="$t('tg-card.users.amountLabel')"><a-input v-model:value="amount" inputmode="decimal" :placeholder="$t('tg-card.users.amountPlaceholder')"/></a-form-item><a-form-item :label="$t('tg-card.users.note')"><a-textarea v-model:value="note" :rows="3" maxlength="500" :placeholder="$t('tg-card.users.notePlaceholder')"/></a-form-item></a-form>
    <div class="text-xs text-gray-500">{{$t('tg-card.users.adjustmentHint')}}</div>
   </template>
  </a-modal>
 </div>
</template>
