<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { useVbenForm } from '#/adapter/form';
import { requestClient } from '#/api/request';
interface Bot{id:string;name:string;username:string;status:number;welcomeText:string;createdAt:string;updatedAt:string}
const loading=ref(false),saving=ref(false),editingId=ref(''),bots=ref<Bot[]>([]),botModalOpen=ref(false);
const {width}=useWindowSize(),isMobile=computed(()=>width.value<768);
const [BotForm,botFormApi]=useVbenForm({
 commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',
 schema:[
  {component:'Input',fieldName:'name',label:'机器人名称',componentProps:{maxlength:100,placeholder:'例如：主商城机器人'}},
  {component:'InputPassword',fieldName:'token',label:'Bot Token',componentProps:{autocomplete:'new-password',placeholder:'从 @BotFather 获取的 Token（编辑时留空）'}},
  {component:'Textarea',fieldName:'welcomeText',label:'欢迎语',componentProps:{rows:4,maxlength:4000,showCount:true,placeholder:'用户首次打开机器人时显示的欢迎内容'}},
  {component:'Switch',fieldName:'enabled',label:'运行开关'},
 ],
});
async function loadBots(){loading.value=true;try{bots.value=await requestClient.get<Bot[]>('/bots')}catch{message.error('机器人列表加载失败')}finally{loading.value=false}}
async function handleAddBot(){editingId.value='';await botFormApi.setValues({name:'',token:'',welcomeText:'欢迎来到我们的商城！',enabled:true});botModalOpen.value=true}
async function openEdit(bot:Bot){editingId.value=bot.id;await botFormApi.setValues({name:bot.name,token:'',welcomeText:bot.welcomeText,enabled:bot.status===1});botModalOpen.value=true}
async function submit(){
 const {valid}=await botFormApi.validate();if(!valid)return;
 const values=await botFormApi.getValues();
 if(!String(values.name||'').trim()){message.warning('请输入机器人名称');return}
 if(!editingId.value&&!String(values.token||'').trim()){message.warning('请输入 Telegram Bot Token');return}
 saving.value=true;
 try{
  if(editingId.value){await requestClient.put('/bots/'+editingId.value,{name:String(values.name).trim(),welcomeText:values.welcomeText||'',status:values.enabled?1:0});message.success('机器人配置已更新')}
  else{await requestClient.post('/bots',{name:String(values.name).trim(),token:String(values.token).trim(),welcomeText:values.welcomeText||''});message.success('机器人已添加，Token 已加密保存')}
  botModalOpen.value=false;await loadBots();
 }catch(error:any){message.error(error?.response?.data?.message||'机器人保存失败')}finally{saving.value=false}
}
function removeBot(bot:Bot){Modal.confirm({title:'删除机器人',content:`确定删除 @${bot.username} 吗？此操作不可撤销。`,okType:'danger',onOk:async()=>{await requestClient.delete('/bots/'+bot.id);message.success('机器人已删除');await loadBots()}})}
onMounted(loadBots);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
  <a-card :bordered="false" :body-style="{ padding: '20px 24px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3"><div class="min-w-0"><div class="text-base font-semibold">Telegram 机器人</div><div class="mt-2 text-xs text-gray-500">管理商城销售入口，所有机器人共享商品、库存和订单</div></div><a-button type="primary" @click="handleAddBot">添加机器人</a-button></div></template>
   <a-alert class="mb-4" type="info" show-icon message="Token 安全说明" description="添加时会实时调用 Telegram getMe 验证 Token，并使用 AES-256-GCM 加密保存。Token 不会在列表中再次显示。"/>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="bot in bots" :key="bot.id" size="small">
     <div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0"><div class="break-words text-sm font-semibold">{{bot.name}}</div><a class="mt-1 block truncate text-sm" :href="`https://t.me/${bot.username}`" target="_blank" rel="noreferrer">@{{bot.username}}</a></div><a-tag class="shrink-0" :color="bot.status===1?'success':'default'">{{bot.status===1?'已启用':'已停用'}}</a-tag></div>
     <div class="mt-3 flex items-center justify-between gap-2"><span class="text-xs text-gray-500">{{bot.createdAt}}</span><a-space><a-button size="small" @click="openEdit(bot)">编辑</a-button><a-button size="small" danger @click="removeBot(bot)">删除</a-button></a-space></div>
    </a-card>
    <a-empty v-if="!loading&&bots.length===0" description="还没有添加机器人"/>
   </div>
   <a-table v-else :data-source="bots" :loading="loading" :pagination="{pageSize:10,showSizeChanger:true}" row-key="id" :scroll="{x:760}">
    <a-table-column key="name" title="机器人名称" data-index="name"/>
    <a-table-column key="username" title="Telegram 用户名"><template #default="{record}"><a :href="`https://t.me/${record.username}`" target="_blank" rel="noreferrer">@{{record.username}}</a></template></a-table-column>
    <a-table-column key="status" title="状态" width="110"><template #default="{record}"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?'已启用':'已停用'}}</a-tag></template></a-table-column>
    <a-table-column key="createdAt" title="创建时间" data-index="createdAt"/>
    <a-table-column key="actions" title="操作" fixed="right" width="150"><template #default="{record}"><a-space><a-button type="link" size="small" @click="openEdit(record)">编辑</a-button><a-button type="link" danger size="small" @click="removeBot(record)">删除</a-button></a-space></template></a-table-column>
   </a-table>
  </a-card>
  <a-modal v-model:open="botModalOpen" title="Telegram 机器人配置" :confirm-loading="saving" :width="560" @ok="submit">
   <div class="p-6"><a-alert v-if="!editingId" class="mb-4" type="info" show-icon message="新 Bot Token 会实时调用 Telegram getMe 验证，并使用 AES-256-GCM 加密保存。"/><BotForm/></div>
  </a-modal>
 </div>
</template>
