<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
interface SiteSettings {
 siteName:string;siteLogo:string;contactEmail:string;smtpHost:string;smtpPort:number;smtpUsername:string;smtpPassword:string;smtpFromName:string;smtpFromEmail:string;smtpUseTLS:boolean;
 emailOrderCreated:boolean;emailPaymentSuccess:boolean;emailDeliverySuccess:boolean;emailSupportCreated:boolean;
 telegramAdminChatId:string;telegramGroupChatId:string;telegramNotifyOrderCreated:boolean;telegramNotifyPaymentSuccess:boolean;telegramNotifyDeliverySuccess:boolean;telegramNotifySupportCreated:boolean;
}
const loading=ref(false),saving=ref(false),testing=ref(false),passwordConfigured=ref(false);
const form=reactive<SiteSettings>({siteName:'TG-Card',siteLogo:'',contactEmail:'',smtpHost:'',smtpPort:587,smtpUsername:'',smtpPassword:'',smtpFromName:'TG-Card',smtpFromEmail:'',smtpUseTLS:true,emailOrderCreated:false,emailPaymentSuccess:true,emailDeliverySuccess:true,emailSupportCreated:true,telegramAdminChatId:'',telegramGroupChatId:'',telegramNotifyOrderCreated:true,telegramNotifyPaymentSuccess:true,telegramNotifyDeliverySuccess:true,telegramNotifySupportCreated:true});
async function load(){loading.value=true;try{const data=await requestClient.get<SiteSettings & {smtpPasswordConfigured:boolean}>('/settings/site');Object.assign(form,data);passwordConfigured.value=!!data.smtpPasswordConfigured;form.smtpPassword='';}catch{message.error('网站设置加载失败');}finally{loading.value=false;}}
async function save(){saving.value=true;try{const data=await requestClient.put<SiteSettings & {smtpPasswordConfigured:boolean}>('/settings/site',form);Object.assign(form,data);form.smtpPassword='';passwordConfigured.value=!!data.smtpPasswordConfigured;message.success('网站设置已保存');}catch(error:any){message.error(error?.response?.data?.message||'保存网站设置失败');}finally{saving.value=false;}}
async function testEmail(){testing.value=true;try{await requestClient.post('/settings/site/test-email',{});message.success('测试邮件已发送，请检查收件箱');}catch(error:any){message.error(error?.response?.data?.message||'测试邮件发送失败');}finally{testing.value=false;}}
async function testTelegram(){testing.value=true;try{await requestClient.post('/settings/site/test-telegram',{});message.success('Telegram 测试通知已发送');}catch(error:any){message.error(error?.response?.data?.message||'Telegram 测试通知失败');}finally{testing.value=false;}}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-6xl flex-col gap-4 p-3 md:p-6">
  <div><h1 class="m-0 text-2xl font-semibold">网站设置</h1><p class="mt-1 text-sm text-gray-500">统一管理站点信息、邮件服务与 Telegram 通知。配置保存到数据库，无需修改服务器环境变量。</p></div>
  <a-card :bordered="false" :loading="loading" title="基础信息"><a-form layout="vertical"><a-row :gutter="16">
   <a-col :xs="24" :md="12"><a-form-item label="站点名称"><a-input v-model:value="form.siteName" maxlength="100"/></a-form-item></a-col>
   <a-col :xs="24" :md="12"><a-form-item label="联系邮箱"><a-input v-model:value="form.contactEmail" type="email"/></a-form-item></a-col>
   <a-col :span="24"><a-form-item label="站点 Logo 地址"><a-input v-model:value="form.siteLogo" placeholder="https://...（可留空）"/></a-form-item></a-col>
  </a-row></a-form></a-card>
  <a-card :bordered="false" title="邮件服务（SMTP）"><a-alert class="mb-4" type="info" show-icon message="SMTP 密码加密保存在数据库。密码留空时保持已保存的密码不变。"/>
   <a-form layout="vertical"><a-row :gutter="16">
    <a-col :xs="24" :md="12"><a-form-item label="SMTP 主机"><a-input v-model:value="form.smtpHost" placeholder="smtp.example.com"/></a-form-item></a-col>
    <a-col :xs="24" :md="12"><a-form-item label="SMTP 端口"><a-input-number v-model:value="form.smtpPort" class="w-full" :min="1" :max="65535"/></a-form-item></a-col>
    <a-col :xs="24" :md="12"><a-form-item label="SMTP 用户名"><a-input v-model:value="form.smtpUsername"/></a-form-item></a-col>
    <a-col :xs="24" :md="12"><a-form-item label="SMTP 密码"><a-input-password v-model:value="form.smtpPassword" autocomplete="new-password" :placeholder="passwordConfigured?'已配置，留空保持不变':'请输入 SMTP 密码'"/></a-form-item></a-col>
    <a-col :xs="24" :md="12"><a-form-item label="发件人名称"><a-input v-model:value="form.smtpFromName"/></a-form-item></a-col>
    <a-col :xs="24" :md="12"><a-form-item label="发件人邮箱"><a-input v-model:value="form.smtpFromEmail" type="email"/></a-form-item></a-col>
   </a-row><a-form-item label="SMTP TLS"><a-switch v-model:checked="form.smtpUseTLS"/></a-form-item></a-form>
   <a-space wrap><a-button :loading="testing" :disabled="!form.contactEmail" @click="testEmail">发送测试邮件到联系邮箱</a-button><a-button :loading="testing" @click="testTelegram">发送 Telegram 测试通知</a-button></a-space>
  </a-card>
  <a-card :bordered="false" title="邮件通知开关"><a-row :gutter="[16,12]">
   <a-col :xs="24" :sm="12"><a-form-item label="新订单通知"><a-switch v-model:checked="form.emailOrderCreated"/></a-form-item></a-col>
   <a-col :xs="24" :sm="12"><a-form-item label="付款成功通知"><a-switch v-model:checked="form.emailPaymentSuccess"/></a-form-item></a-col>
   <a-col :xs="24" :sm="12"><a-form-item label="自动发货通知"><a-switch v-model:checked="form.emailDeliverySuccess"/></a-form-item></a-col>
   <a-col :xs="24" :sm="12"><a-form-item label="售后工单通知"><a-switch v-model:checked="form.emailSupportCreated"/></a-form-item></a-col>
  </a-row></a-card>
  <a-card :bordered="false" title="Telegram 私聊与群组通知"><a-alert class="mb-4" type="info" show-icon message="填写管理员私聊和通知群组 Chat ID；对应 Bot 必须能向该目标发送消息。"/>
   <a-form layout="vertical"><a-row :gutter="16">
    <a-col :xs="24" :md="12"><a-form-item label="管理员私聊 Chat ID"><a-input v-model:value="form.telegramAdminChatId"/></a-form-item></a-col>
    <a-col :xs="24" :md="12"><a-form-item label="通知群组 Chat ID"><a-input v-model:value="form.telegramGroupChatId"/></a-form-item></a-col>
   </a-row></a-form><a-divider orientation="left">通知事件</a-divider><a-row :gutter="[16,12]">
    <a-col :xs="24" :sm="12"><a-form-item label="新订单"><a-switch v-model:checked="form.telegramNotifyOrderCreated"/></a-form-item></a-col>
    <a-col :xs="24" :sm="12"><a-form-item label="付款成功"><a-switch v-model:checked="form.telegramNotifyPaymentSuccess"/></a-form-item></a-col>
    <a-col :xs="24" :sm="12"><a-form-item label="发货成功"><a-switch v-model:checked="form.telegramNotifyDeliverySuccess"/></a-form-item></a-col>
    <a-col :xs="24" :sm="12"><a-form-item label="售后工单"><a-switch v-model:checked="form.telegramNotifySupportCreated"/></a-form-item></a-col>
   </a-row>
  </a-card>
  <div class="flex flex-wrap justify-end gap-2"><a-button :loading="loading" @click="load">重新加载</a-button><a-button type="primary" :loading="saving" @click="save">保存网站设置</a-button></div>
 </div>
</template>
