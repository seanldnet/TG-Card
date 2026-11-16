<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
interface SiteSettings {
 siteName:string;siteLogo:string;contactEmail:string;telegramCustomerServiceUrl:string;smtpHost:string;smtpPort:number;smtpUsername:string;smtpPassword:string;smtpFromName:string;smtpFromEmail:string;smtpUseTLS:boolean;
 emailOrderCreated:boolean;emailPaymentSuccess:boolean;emailDeliverySuccess:boolean;emailSupportCreated:boolean;
 telegramAdminChatId:string;telegramGroupChatId:string;telegramNotifyOrderCreated:boolean;telegramNotifyPaymentSuccess:boolean;telegramNotifyDeliverySuccess:boolean;telegramNotifySupportCreated:boolean;
}
const loading=ref(false),saving=ref(false),testing=ref(false),logoUploading=ref(false),passwordConfigured=ref(false),activeTab=ref('basic');
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const form=reactive<SiteSettings>({siteName:'TG-Card',siteLogo:'',contactEmail:'',telegramCustomerServiceUrl:'',smtpHost:'',smtpPort:587,smtpUsername:'',smtpPassword:'',smtpFromName:'TG-Card',smtpFromEmail:'',smtpUseTLS:true,emailOrderCreated:false,emailPaymentSuccess:true,emailDeliverySuccess:true,emailSupportCreated:true,telegramAdminChatId:'',telegramGroupChatId:'',telegramNotifyOrderCreated:true,telegramNotifyPaymentSuccess:true,telegramNotifyDeliverySuccess:true,telegramNotifySupportCreated:true});
async function load(){loading.value=true;try{const data=await requestClient.get<SiteSettings & {smtpPasswordConfigured:boolean}>('/settings/site');Object.assign(form,data);passwordConfigured.value=!!data.smtpPasswordConfigured;form.smtpPassword='';}catch{message.error('网站设置加载失败');}finally{loading.value=false;}}
async function save(){saving.value=true;try{const data=await requestClient.put<SiteSettings & {smtpPasswordConfigured:boolean}>('/settings/site',form);Object.assign(form,data);form.smtpPassword='';passwordConfigured.value=!!data.smtpPasswordConfigured;message.success('网站设置已保存');}catch(error:any){message.error(error?.response?.data?.message||'保存网站设置失败');}finally{saving.value=false;}}
async function uploadSiteLogo(file:any){const image=file?.originFileObj||file;if(!(image instanceof Blob)){message.error('无法读取图标文件');return false;}if(image.size>10*1024*1024){message.warning('网站图标不能超过 10 MB');return false;}logoUploading.value=true;try{const body=new FormData();body.append('file',image,image.name||'site-logo');const result=await requestClient.post<{siteLogo:string}>('/settings/site/logo',body);form.siteLogo=result.siteLogo;message.success('网站图标上传成功，请保存设置');}catch(error:any){message.error(error?.response?.data?.message||'网站图标上传失败');}finally{logoUploading.value=false;}return false;}
async function testEmail(){testing.value=true;try{await requestClient.post('/settings/site/test-email',{});message.success('测试邮件已发送，请检查收件箱');}catch(error:any){message.error(error?.response?.data?.message||'测试邮件发送失败');}finally{testing.value=false;}}
async function testTelegram(){testing.value=true;try{await requestClient.post('/settings/site/test-telegram',{});message.success('Telegram 测试通知已发送');}catch(error:any){message.error(error?.response?.data?.message||'Telegram 测试通知失败');}finally{testing.value=false;}}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-6xl flex-col gap-4 p-3 md:p-6">
  <div><h1 class="m-0 text-xl font-semibold">网站设置</h1><p class="mb-0 mt-1 text-xs leading-5 text-gray-500">按类别分开管理。配置会保存到数据库，无需修改服务器环境变量。</p></div>
  <a-card :bordered="false" :loading="loading" :body-style="{padding:'16px'}">
   <a-tabs v-model:activeKey="activeTab" :tab-position="isMobile?'top':'left'" :tab-bar-gutter="12" class="site-settings-tabs">
    <a-tab-pane key="basic">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:web" />基础信息</span></template>
     <div class="max-w-3xl">
      <div class="mb-4 text-sm font-semibold">站点基础信息</div>
      <a-form layout="vertical"><a-row :gutter="16">
       <a-col :xs="24" :md="12"><a-form-item label="站点名称"><a-input v-model:value="form.siteName" maxlength="100" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="Telegram 客服链接"><a-input v-model:value="form.telegramCustomerServiceUrl" placeholder="https://t.me/客服用户名" /></a-form-item></a-col>
       <a-col :span="24"><a-form-item label="网站图标"><div class="flex flex-wrap items-center gap-3"><img v-if="form.siteLogo" :src="form.siteLogo" alt="网站图标预览" class="h-14 w-14 rounded-lg border border-gray-200 object-contain p-1" /><a-upload accept="image/*" :show-upload-list="false" :before-upload="uploadSiteLogo"><a-button :loading="logoUploading">上传网站图标</a-button></a-upload><span class="text-xs text-gray-500">支持常见图片格式，最大 10 MB</span></div></a-form-item></a-col>
      </a-row></a-form>
     </div>
    </a-tab-pane>
    <a-tab-pane key="smtp">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:email-outline" />邮件服务</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">SMTP 邮件服务</div>
      <a-alert class="mb-4" type="info" show-icon message="SMTP 密码加密保存在数据库。密码留空时保持已保存的密码不变。" />
      <a-form layout="vertical"><a-row :gutter="16">
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 主机"><a-input v-model:value="form.smtpHost" placeholder="smtp.example.com" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 端口"><a-input-number v-model:value="form.smtpPort" class="w-full" :min="1" :max="65535" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 用户名"><a-input v-model:value="form.smtpUsername" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 密码"><a-input-password v-model:value="form.smtpPassword" autocomplete="new-password" :placeholder="passwordConfigured?'已配置，留空保持不变':'请输入 SMTP 密码'" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="发件人名称"><a-input v-model:value="form.smtpFromName" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="发件人邮箱"><a-input v-model:value="form.smtpFromEmail" type="email" /></a-form-item></a-col>
      </a-row><a-form-item label="SMTP TLS"><a-switch v-model:checked="form.smtpUseTLS" /></a-form-item></a-form>
      <a-form-item label="邮件通知收件邮箱（用于测试邮件及业务邮件通知）"><a-input v-model:value="form.contactEmail" type="email" placeholder="可选，填写接收通知的邮箱" /></a-form-item>
      <a-space wrap><a-button size="small" :loading="testing" :disabled="!form.contactEmail" @click="testEmail">发送测试邮件</a-button></a-space>
     </div>
    </a-tab-pane>
    <a-tab-pane key="email">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:bell-outline" />邮件通知</span></template>
     <div class="max-w-3xl">
      <div class="mb-4 text-sm font-semibold">邮件通知开关</div>
      <a-alert class="mb-4" type="info" show-icon message="选择哪些业务事件需要发送邮件通知。" />
      <a-row :gutter="[16,12]">
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">新订单通知</span><a-switch v-model:checked="form.emailOrderCreated" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">付款成功通知</span><a-switch v-model:checked="form.emailPaymentSuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">自动发货通知</span><a-switch v-model:checked="form.emailDeliverySuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">售后工单通知</span><a-switch v-model:checked="form.emailSupportCreated" /></div></a-col>
      </a-row>
     </div>
    </a-tab-pane>
    <a-tab-pane key="telegram">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:telegram" />Telegram 通知</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">Telegram 私聊与群组通知</div>
      <a-alert class="mb-4" type="info" show-icon message="填写管理员私聊和通知群组 Chat ID；对应 Bot 必须能向该目标发送消息。" />
      <a-form layout="vertical"><a-row :gutter="16">
       <a-col :xs="24" :md="12"><a-form-item label="管理员私聊 Chat ID"><a-input v-model:value="form.telegramAdminChatId" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="通知群组 Chat ID"><a-input v-model:value="form.telegramGroupChatId" /></a-form-item></a-col>
      </a-row></a-form>
      <div class="mb-3 mt-2 text-sm font-semibold">通知事件</div>
      <a-row :gutter="[16,12]">
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">新订单</span><a-switch v-model:checked="form.telegramNotifyOrderCreated" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">付款成功</span><a-switch v-model:checked="form.telegramNotifyPaymentSuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">发货成功</span><a-switch v-model:checked="form.telegramNotifyDeliverySuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">售后工单</span><a-switch v-model:checked="form.telegramNotifySupportCreated" /></div></a-col>
      </a-row>
      <a-button class="mt-4" size="small" :loading="testing" @click="testTelegram">发送 Telegram 测试通知</a-button>
     </div>
    </a-tab-pane>
   </a-tabs>
  </a-card>
  <div class="flex flex-wrap items-center justify-between gap-2">
   <span class="text-xs text-gray-400">切换分组不会自动保存，请完成修改后点击保存。</span>
   <a-space><a-button size="small" :loading="loading" @click="load">重新加载</a-button><a-button size="middle" type="primary" :loading="saving" @click="save">保存设置</a-button></a-space>
  </div>
 </div>
</template>