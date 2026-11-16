<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';

interface Bot { id:string; name:string; username:string; status:number; welcomeText:string; welcomeMessage:string; purchaseNotice:string; description:string; ownerId:string; customerServiceUrl:string; businessGroupUrl:string; createdAt:string; updatedAt:string }
const loading=ref(false),saving=ref(false),botModalOpen=ref(false),editingId=ref('');
const bots=ref<Bot[]>([]);
const botFormRef=ref();
const botForm=reactive({name:'',token:'',welcomeText:'欢迎来到我们的商城！',welcomeMessage:'🎉 欢迎来到我们的商城！',purchaseNotice:'📌 购买须知：\n• 请确认商品信息及库存后再下单\n• 虚拟商品发货后请妥善保管卡密',description:'欢迎使用本机器人，浏览商品并安全快捷地完成购买。',ownerId:'',customerServiceUrl:'',businessGroupUrl:'',enabled:true});
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const activeCount=computed(()=>bots.value.filter(item=>item.status===1).length);
const disabledCount=computed(()=>bots.value.length-activeCount.value);
const botRules={
 name:[{required:true,whitespace:true,message:'请输入机器人名称',trigger:'blur'}],
 token:[{validator:(_rule:any,value:string)=>editingId.value||String(value||'').trim()?Promise.resolve():Promise.reject(new Error('请输入 Telegram Bot Token')),trigger:'blur'}],
};
function resetBotForm(){botForm.name='';botForm.token='';botForm.welcomeText='欢迎来到我们的商城！';botForm.welcomeMessage='🎉 欢迎来到我们的商城！';botForm.purchaseNotice='📌 购买须知：\\n• 请确认商品信息及库存后再下单\\n• 虚拟商品发货后请妥善保管卡密';botForm.description='欢迎使用本机器人，浏览商品并安全快捷地完成购买。';botForm.ownerId='';botForm.customerServiceUrl='';botForm.businessGroupUrl='';botForm.enabled=true}
function handleAddBot(){editingId.value='';resetBotForm();botModalOpen.value=true}
function openEdit(bot:Bot){editingId.value=bot.id;botForm.name=bot.name;botForm.token='';botForm.welcomeText=bot.welcomeText||'';botForm.welcomeMessage=bot.welcomeMessage||'🎉 欢迎来到我们的商城！';botForm.purchaseNotice=bot.purchaseNotice||'📌 购买须知：\\n• 请确认商品信息及库存后再下单\\n• 虚拟商品发货后请妥善保管卡密';botForm.description=bot.description||'欢迎使用本机器人，浏览商品并安全快捷地完成购买。';botForm.ownerId=bot.ownerId||'';botForm.customerServiceUrl=bot.customerServiceUrl||'';botForm.businessGroupUrl=bot.businessGroupUrl||'';botForm.enabled=bot.status===1;botModalOpen.value=true}
async function loadBots(){loading.value=true;try{bots.value=await requestClient.get<Bot[]>('/bots')}catch(error:any){message.error(error?.response?.data?.message||'机器人列表加载失败')}finally{loading.value=false}}
async function submit(){
 try{await botFormRef.value?.validate()}catch{return}
 if(!editingId.value&&!botForm.token.trim()){message.warning('请填写 Bot Token');return}
 saving.value=true;
 try{
  if(editingId.value){
   await requestClient.put('/bots/'+editingId.value,{name:botForm.name.trim(),welcomeText:botForm.welcomeText,status:botForm.enabled?1:0,welcomeMessage:botForm.welcomeMessage,purchaseNotice:botForm.purchaseNotice,description:botForm.description,ownerId:botForm.ownerId.trim(),customerServiceUrl:botForm.customerServiceUrl.trim(),businessGroupUrl:botForm.businessGroupUrl.trim()});
   message.success('机器人资料已保存');
  }else{
   await requestClient.post('/bots',{name:botForm.name.trim(),token:botForm.token.trim(),welcomeText:botForm.welcomeText,welcomeMessage:botForm.welcomeMessage,purchaseNotice:botForm.purchaseNotice,description:botForm.description,ownerId:botForm.ownerId.trim(),customerServiceUrl:botForm.customerServiceUrl.trim(),businessGroupUrl:botForm.businessGroupUrl.trim()});
   message.success('机器人添加成功');
  }
  botModalOpen.value=false;await loadBots();
 }catch(error:any){message.error(error?.response?.data?.message||error?.message||'保存失败，请检查填写内容')}
 finally{saving.value=false}
}
function removeBot(bot:Bot){Modal.confirm({title:'删除机器人',content:'确定删除 @'+bot.username+' 吗？此操作不可撤销。',okType:'danger',onOk:async()=>{try{await requestClient.delete('/bots/'+bot.id);message.success('机器人已删除');await loadBots()}catch(error:any){message.error(error?.response?.data?.message||'删除失败')}}})}
onMounted(loadBots);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-5 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div class="min-w-0"><h1 class="m-0 text-xl font-semibold tracking-tight">机器人管理</h1><p class="mb-0 mt-1 text-xs text-gray-500">统一管理 Telegram 商城机器人及运行状态</p></div>
   <div class="flex w-full flex-wrap items-center gap-2 sm:w-auto">
<a-button type="primary" size="middle" @click="handleAddBot">＋添加机器人</a-button>
   </div>
  </div>
  <a-row :gutter="[12,12]">
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">机器人总数</div><div class="mt-2 text-2xl font-semibold leading-none">{{bots.length}}</div><div class="mt-2 text-xs text-gray-400">已添加的机器人</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:robot-outline" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">运行中</div><div class="mt-2 text-2xl font-semibold leading-none">{{activeCount}}</div><div class="mt-2 text-xs text-gray-400">正常启用的机器人</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:check-circle-outline" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">已停用</div><div class="mt-2 text-2xl font-semibold leading-none">{{disabledCount}}</div><div class="mt-2 text-xs text-gray-400">当前未运行的机器人</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:pause-circle-outline" class="block text-2xl" /></span></div></a-card></a-col>
  </a-row>
  <a-alert type="info" show-icon message="添加机器人需要填写 BotFather 提供的 Token。系统会先向 Telegram 验证，再加密保存。" />
  <a-card :bordered="false" class="rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}">
   <div class="mb-4 flex flex-wrap items-center justify-between gap-2"><div><div class="text-sm font-semibold">机器人列表</div><div class="mt-1 text-xs text-gray-400">管理机器人名称、欢迎语和启用状态</div></div><a-button size="small" :loading="loading" @click="loadBots">刷新列表</a-button></div>
   <div v-if="isMobile" class="flex flex-col gap-3">
    <a-card v-for="bot in bots" :key="bot.id" size="small" class="rounded-lg border border-gray-100" :body-style="{padding:'20px 24px'}">
     <div class="flex min-w-0 items-start justify-between gap-3"><div class="min-w-0"><div class="truncate text-sm font-semibold">{{bot.name}}</div><a class="mt-1 block truncate text-xs" :href="'https://t.me/'+bot.username" target="_blank" rel="noreferrer">@{{bot.username}}</a></div><a-tag class="m-0 shrink-0" :color="bot.status===1?'success':'default'">{{bot.status===1?'运行中':'已停用'}}</a-tag></div>
     <a-divider class="my-3"/><div class="flex items-center justify-between gap-2"><span class="truncate text-xs text-gray-400">{{bot.createdAt}}</span><a-space size="small"><a-button size="small" @click="openEdit(bot)">编辑</a-button><a-button size="small" danger @click="removeBot(bot)">删除</a-button></a-space></div>
    </a-card>
    <a-empty v-if="!loading&&bots.length===0" description="还没有机器人，点击右上角添加"/>
   </div>
   <a-table v-else :data-source="bots" :loading="loading" :pagination="{pageSize:10,showSizeChanger:true}" row-key="id" :scroll="{x:760}">
    <a-table-column key="name" title="机器人名称" data-index="name"/>
    <a-table-column key="username" title="Telegram 用户名"><template #default="{record}"><a :href="'https://t.me/'+record.username" target="_blank" rel="noreferrer">@{{record.username}}</a></template></a-table-column>
    <a-table-column key="status" title="状态" width="110"><template #default="{record}"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?'运行中':'已停用'}}</a-tag></template></a-table-column>
    <a-table-column key="createdAt" title="创建时间" data-index="createdAt"/>
    <a-table-column key="actions" title="操作" fixed="right" width="150"><template #default="{record}"><a-space><a-button type="link" size="small" @click="openEdit(record)">编辑</a-button><a-button type="link" danger size="small" @click="removeBot(record)">删除</a-button></a-space></template></a-table-column>
    <template #emptyText><a-empty description="还没有机器人，点击添加机器人开始使用"/></template>
   </a-table>
  </a-card>
  <a-modal v-model:open="botModalOpen" :title="editingId?'编辑机器人':'添加机器人'" :width="560" :confirm-loading="saving" ok-text="保存" cancel-text="取消" @ok="submit">
   <div class="px-1 py-3">
    <a-alert v-if="!editingId" class="mb-4" type="info" show-icon message="请粘贴完整的 Telegram Bot Token，系统会自动验证并安全保存。"/>
    <a-form ref="botFormRef" :model="botForm" :rules="botRules" layout="vertical">
     <a-form-item label="机器人名称" name="name"><a-input v-model:value="botForm.name" size="large" :maxlength="100" placeholder="例如：主商城机器人"/></a-form-item>
     <a-form-item label="Bot Token" name="token" :extra="editingId?'留空表示不修改现有 Token。':'从 Telegram 的 @BotFather 获取。'"><a-input-password v-model:value="botForm.token" size="large" autocomplete="new-password" placeholder="输入 Bot Token"/></a-form-item>
     <a-form-item label="机器人资料介绍"><a-textarea v-model:value="botForm.description" :rows="3" :maxlength="512" show-count placeholder="用户点开机器人资料时看到的介绍"/></a-form-item>
     <a-divider class="my-2">/start 欢迎页内容</a-divider>
     <a-form-item label="上方欢迎文案"><a-textarea v-model:value="botForm.welcomeMessage" :rows="3" :maxlength="2000" show-count placeholder="支持自定义欢迎介绍；Telegram ID 和余额由系统自动显示"/></a-form-item>
     <a-form-item label="下方购买须知"><a-textarea v-model:value="botForm.purchaseNotice" :rows="4" :maxlength="2000" show-count placeholder="支持自定义购买须知"/></a-form-item>
     <a-divider class="my-2">机器人专属设置</a-divider>
     <a-form-item label="主人 Telegram 数字 ID"><a-input v-model:value="botForm.ownerId" inputmode="numeric" placeholder="填写此机器人的主人 ID"/></a-form-item>
     <a-form-item label="客服链接"><a-input v-model:value="botForm.customerServiceUrl" placeholder="https://t.me/客服用户名"/></a-form-item>
     <a-form-item label="业务通知群链接"><a-input v-model:value="botForm.businessGroupUrl" placeholder="https://t.me/业务通知群"/></a-form-item>
     <a-form-item label="运行状态"><a-switch v-model:checked="botForm.enabled" checked-children="启用" un-checked-children="停用"/></a-form-item>
    </a-form>
   </div>
  </a-modal>
 </div>
</template>