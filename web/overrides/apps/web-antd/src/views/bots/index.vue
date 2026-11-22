<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface RuntimeStatus { botId:string; username:string; state:string; running:boolean; lastPollAt?:string; lastUpdateAt?:string; lastError?:string; lastErrorAt?:string }
interface Bot { id:string; name:string; username:string; status:number; welcomeText:string; welcomeMessage:string; purchaseNotice:string; description:string; ownerId:string; customerServiceUrl:string; businessGroupUrl:string; createdAt:string; updatedAt:string }
const loading=ref(false),saving=ref(false),botModalOpen=ref(false),editingId=ref('');
const bots=ref<Bot[]>([]),runtimeStatuses=ref<RuntimeStatus[]>([]);
let runtimeTimer:ReturnType<typeof setInterval>|undefined;
const runtimeByBot=computed(()=>Object.fromEntries(runtimeStatuses.value.map(item=>[item.botId,item])));
const botFormRef=ref();
const botForm=reactive({name:'',token:'',welcomeText:'欢迎来到我们的商城！',welcomeMessage:'🎉 欢迎来到我们的商城！',purchaseNotice:'📌 购买须知：\n• 请确认商品信息及库存后再下单\n• 虚拟商品发货后请妥善保管卡密',description:'欢迎使用本机器人，浏览商品并安全快捷地完成购买。',ownerId:'',customerServiceUrl:'',businessGroupUrl:'',enabled:true});
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const activeCount=computed(()=>bots.value.filter(item=>item.status===1).length);
const disabledCount=computed(()=>bots.value.length-activeCount.value);
function runtimeStateLabel(state:string){const key='tg-card.bots.'+state;const value=$t(key);return value===key?$t('tg-card.bots.waiting'):value}
const botRules={
 name:[{required:true,whitespace:true,message:$t('tg-card.bots.nameRequired'),trigger:'blur'}],
 token:[{validator:(_rule:any,value:string)=>editingId.value||String(value||'').trim()?Promise.resolve():Promise.reject(new Error($t('tg-card.bots.tokenRequired'))),trigger:'blur'}],
};
function resetBotForm(){botForm.name='';botForm.token='';botForm.welcomeText='欢迎来到我们的商城！';botForm.welcomeMessage='🎉 欢迎来到我们的商城！';botForm.purchaseNotice='📌 购买须知：\\n• 请确认商品信息及库存后再下单\\n• 虚拟商品发货后请妥善保管卡密';botForm.description='欢迎使用本机器人，浏览商品并安全快捷地完成购买。';botForm.ownerId='';botForm.customerServiceUrl='';botForm.businessGroupUrl='';botForm.enabled=true}
function handleAddBot(){editingId.value='';resetBotForm();botModalOpen.value=true}
function openEdit(bot:Bot){editingId.value=bot.id;botForm.name=bot.name;botForm.token='';botForm.welcomeText=bot.welcomeText||'';botForm.welcomeMessage=bot.welcomeMessage||'🎉 欢迎来到我们的商城！';botForm.purchaseNotice=bot.purchaseNotice||'📌 购买须知：\\n• 请确认商品信息及库存后再下单\\n• 虚拟商品发货后请妥善保管卡密';botForm.description=bot.description||'欢迎使用本机器人，浏览商品并安全快捷地完成购买。';botForm.ownerId=bot.ownerId||'';botForm.customerServiceUrl=bot.customerServiceUrl||'';botForm.businessGroupUrl=bot.businessGroupUrl||'';botForm.enabled=bot.status===1;botModalOpen.value=true}
async function refreshRuntime(){try{runtimeStatuses.value=await requestClient.get<RuntimeStatus[]>('/bots/runtime')}catch{/* Keep the last known runtime snapshot; bot list remains usable. */}}
async function restartBot(bot:Bot){
 Modal.confirm({title:$t('tg-card.bots.restartTitle'),content:$t('tg-card.bots.restartConfirm',{username:bot.username}),okText:$t('tg-card.bots.confirmRestart'),cancelText:$t('tg-card.bots.cancel'),onOk:async()=>{
  try{await requestClient.post('/bots/'+bot.id+'/restart');message.success($t('tg-card.bots.restartSubmitted'));await refreshRuntime()}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.bots.restartFailed'))}
 }});
}
async function loadBots(){loading.value=true;try{bots.value=await requestClient.get<Bot[]>('/bots');await refreshRuntime()}catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.bots.listFailed'))}finally{loading.value=false}}
async function submit(){
 try{await botFormRef.value?.validate()}catch{return}
 if(!editingId.value&&!botForm.token.trim()){message.warning($t('tg-card.bots.fillToken'));return}
 saving.value=true;
 try{
  if(editingId.value){
   await requestClient.put('/bots/'+editingId.value,{name:botForm.name.trim(),welcomeText:botForm.welcomeText,status:botForm.enabled?1:0,welcomeMessage:botForm.welcomeMessage,purchaseNotice:botForm.purchaseNotice,description:botForm.description,ownerId:botForm.ownerId.trim(),customerServiceUrl:botForm.customerServiceUrl.trim(),businessGroupUrl:botForm.businessGroupUrl.trim()});
   message.success($t('tg-card.bots.saved'));
  }else{
   await requestClient.post('/bots',{name:botForm.name.trim(),token:botForm.token.trim(),welcomeText:botForm.welcomeText,welcomeMessage:botForm.welcomeMessage,purchaseNotice:botForm.purchaseNotice,description:botForm.description,ownerId:botForm.ownerId.trim(),customerServiceUrl:botForm.customerServiceUrl.trim(),businessGroupUrl:botForm.businessGroupUrl.trim()});
   message.success($t('tg-card.bots.added'));
  }
  botModalOpen.value=false;await loadBots();
 }catch(error:any){message.error(error?.response?.data?.message||error?.message||$t('tg-card.bots.saveFailed'))}
 finally{saving.value=false}
}
function removeBot(bot:Bot){Modal.confirm({title:$t('tg-card.bots.deleteTitle'),content:$t('tg-card.bots.deleteConfirm',{username:bot.username}),okType:'danger',onOk:async()=>{try{await requestClient.delete('/bots/'+bot.id);message.success($t('tg-card.bots.deleted'));await loadBots()}catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.bots.deleteFailed'))}}})}
onMounted(()=>{void loadBots();runtimeTimer=setInterval(()=>void refreshRuntime(),15000)});
onUnmounted(()=>{if(runtimeTimer)clearInterval(runtimeTimer)});
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-5 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div class="min-w-0"><h1 class="m-0 text-xl font-semibold tracking-tight">{{$t('tg-card.bots.title')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.bots.subtitle')}}</p></div>
   <div class="flex w-full flex-wrap items-center gap-2 sm:w-auto">
<a-button type="primary" size="middle" @click="handleAddBot">{{$t('tg-card.bots.add')}}</a-button>
   </div>
  </div>
  <a-row :gutter="[12,12]">
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">{{$t('tg-card.bots.total')}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{bots.length}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.bots.addedCount')}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:robot-outline" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">{{$t('tg-card.bots.running')}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{activeCount}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.bots.enabledCount')}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:check-circle-outline" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">{{$t('tg-card.bots.disabled')}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{disabledCount}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.bots.notRunning')}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:pause-circle-outline" class="block text-2xl" /></span></div></a-card></a-col>
  </a-row>
  <a-alert type="info" show-icon :message="$t('tg-card.bots.tokenInfo')" />
  <a-card :bordered="false" class="rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}">
   <div class="mb-4 flex flex-wrap items-center justify-between gap-2"><div><div class="text-sm font-semibold">{{$t('tg-card.bots.list')}}</div><div class="mt-1 text-xs text-gray-400">{{$t('tg-card.bots.listSubtitle')}}</div></div><a-space><a-button size="small" :loading="loading" @click="loadBots">{{$t('tg-card.bots.refreshList')}}</a-button><a-button size="small" @click="refreshRuntime">{{$t('tg-card.bots.refreshRuntime')}}</a-button></a-space></div>
   <div v-if="isMobile" class="flex flex-col gap-3">
    <a-card v-for="bot in bots" :key="bot.id" size="small" class="rounded-lg border border-gray-100" :body-style="{padding:'20px 24px'}">
     <div class="flex min-w-0 items-start justify-between gap-3"><div class="min-w-0"><div class="truncate text-sm font-semibold">{{bot.name}}</div><a class="mt-1 block truncate text-xs" :href="'https://t.me/'+bot.username" target="_blank" rel="noreferrer">@{{bot.username}}</a></div><a-tag class="m-0 shrink-0" :color="bot.status===1?'success':'default'">{{bot.status!==1?$t('tg-card.bots.stopped'):runtimeStateLabel(runtimeByBot[bot.id]?.state||'waiting')}}</a-tag></div>
     <div v-if="runtimeByBot[bot.id]?.lastError" class="mt-2 break-words text-xs text-red-500">{{runtimeByBot[bot.id]?.lastError}}</div><div v-if="runtimeByBot[bot.id]?.lastPollAt" class="mt-1 text-xs text-gray-400">{{$t('tg-card.bots.lastPoll')}}{{runtimeByBot[bot.id]?.lastPollAt}}</div><a-divider class="my-3"/><div class="flex items-center justify-between gap-2"><span class="truncate text-xs text-gray-400">{{bot.createdAt}}</span><a-space size="small"><a-button size="small" @click="restartBot(bot)">{{$t('tg-card.bots.restart')}}</a-button><a-button size="small" @click="openEdit(bot)">{{$t('tg-card.bots.edit')}}</a-button><a-button size="small" danger @click="removeBot(bot)">{{$t('tg-card.bots.delete')}}</a-button></a-space></div>
    </a-card>
    <a-empty v-if="!loading&&bots.length===0" :description="$t('tg-card.bots.empty')"/>
   </div>
   <a-table v-else :data-source="bots" :loading="loading" :pagination="{pageSize:10,showSizeChanger:true}" row-key="id" :scroll="{x:760}">
    <a-table-column key="name" :title="$t('tg-card.bots.tableName')" data-index="name"/>
    <a-table-column key="username" :title="$t('tg-card.bots.username')"><template #default="{record}"><a :href="'https://t.me/'+record.username" target="_blank" rel="noreferrer">@{{record.username}}</a></template></a-table-column>
    <a-table-column key="status" :title="$t('tg-card.bots.runtime')" width="130"><template #default="{record}"><a-tag :color="record.status!==1?'default':runtimeByBot[record.id]?.state==='online'?'success':runtimeByBot[record.id]?.state==='conflict'?'error':'warning'">{{record.status!==1?$t('tg-card.bots.stopped'):runtimeStateLabel(runtimeByBot[record.id]?.state||'waiting')}}</a-tag><div v-if="runtimeByBot[record.id]?.lastError" class="mt-1 max-w-48 truncate text-xs text-red-500" :title="runtimeByBot[record.id]?.lastError">{{runtimeByBot[record.id]?.lastError}}</div></template></a-table-column>
    <a-table-column key="createdAt" :title="$t('tg-card.bots.createdAt')" data-index="createdAt"/>
    <a-table-column key="actions" :title="$t('tg-card.bots.actions')" fixed="right" width="150"><template #default="{record}"><a-space><a-button type="link" size="small" @click="restartBot(record)">{{$t('tg-card.bots.restart')}}</a-button><a-button type="link" size="small" @click="openEdit(record)">{{$t('tg-card.bots.edit')}}</a-button><a-button type="link" danger size="small" @click="removeBot(record)">{{$t('tg-card.bots.delete')}}</a-button></a-space></template></a-table-column>
    <template #emptyText><a-empty :description="$t('tg-card.bots.emptyHint')"/></template>
   </a-table>
  </a-card>
  <a-modal v-model:open="botModalOpen" :title="editingId?$t('tg-card.bots.editTitle'):$t('tg-card.bots.addTitle')" :width="560" :confirm-loading="saving" :ok-text="$t('tg-card.bots.save')" :cancel-text="$t('tg-card.bots.cancel')" @ok="submit">
   <div class="px-1 py-3">
    <a-alert v-if="!editingId" class="mb-4" type="info" show-icon :message="$t('tg-card.bots.tokenAlert')"/>
    <a-form ref="botFormRef" :model="botForm" :rules="botRules" layout="vertical">
     <a-form-item :label="$t('tg-card.bots.tableName')" name="name"><a-input v-model:value="botForm.name" size="large" :maxlength="100" :placeholder="$t('tg-card.bots.namePlaceholder')"/></a-form-item>
     <a-form-item label="Bot Token" name="token" :extra="editingId?$t('tg-card.bots.tokenEditHint'):$t('tg-card.bots.tokenSource')"><a-input-password v-model:value="botForm.token" size="large" autocomplete="new-password" :placeholder="$t('tg-card.bots.tokenPlaceholder')"/></a-form-item>
     <a-form-item :label="$t('tg-card.bots.description')"><a-textarea v-model:value="botForm.description" :rows="3" :maxlength="512" show-count :placeholder="$t('tg-card.bots.descriptionPlaceholder')"/></a-form-item>
     <a-divider class="my-2">{{$t('tg-card.bots.welcomeContent')}}</a-divider>
     <a-form-item :label="$t('tg-card.bots.welcomeMessage')"><a-textarea v-model:value="botForm.welcomeMessage" :rows="3" :maxlength="2000" show-count :placeholder="$t('tg-card.bots.welcomePlaceholder')"/></a-form-item>
     <a-form-item :label="$t('tg-card.bots.purchaseNotice')"><a-textarea v-model:value="botForm.purchaseNotice" :rows="4" :maxlength="2000" show-count :placeholder="$t('tg-card.bots.purchasePlaceholder')"/></a-form-item>
     <a-divider class="my-2">{{$t('tg-card.bots.settings')}}</a-divider>
     <a-form-item :label="$t('tg-card.bots.ownerId')"><a-input v-model:value="botForm.ownerId" inputmode="numeric" :placeholder="$t('tg-card.bots.ownerPlaceholder')"/></a-form-item>
     <a-form-item :label="$t('tg-card.bots.customerService')"><a-input v-model:value="botForm.customerServiceUrl" :placeholder="$t('tg-card.bots.customerServicePlaceholder')"/></a-form-item>
     <a-form-item :label="$t('tg-card.bots.businessGroup')"><a-input v-model:value="botForm.businessGroupUrl" :placeholder="$t('tg-card.bots.businessGroupPlaceholder')"/></a-form-item>
     <a-form-item :label="$t('tg-card.bots.runtime')"><a-switch v-model:checked="botForm.enabled" :checked-children="$t('tg-card.bots.enable')" :un-checked-children="$t('tg-card.bots.disable')"/></a-form-item>
    </a-form>
   </div>
  </a-modal>
 </div>
</template>