<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';
interface SiteSettings {
 siteName:string;siteLogo:string;contactEmail:string;smtpHost:string;smtpPort:number;smtpUsername:string;smtpPassword:string;smtpFromName:string;smtpFromEmail:string;smtpUseTLS:boolean;
 emailOrderCreated:boolean;emailPaymentSuccess:boolean;emailDeliverySuccess:boolean;emailSupportCreated:boolean;
 telegramAdminChatId:string;telegramGroupChatId:string;telegramNotifyOrderCreated:boolean;telegramNotifyPaymentSuccess:boolean;telegramNotifyDeliverySuccess:boolean;telegramNotifySupportCreated:boolean;telegramNotifyStockLow:boolean;telegramNotifyDailyReport:boolean;telegramNotifyRiskAlert:boolean;stockAutoUnlistOnZero:boolean;stockAutoRelistOnRestock:boolean;
 capEnabled:boolean;capDomain:string;capSiteKey:string;capSecret:string;backupEnabled:boolean;backupTime:string;
}
const loading=ref(false),saving=ref(false),testing=ref(false),logoUploading=ref(false),passwordConfigured=ref(false),capSecretConfigured=ref(false),backupEncryptionKeyConfigured=ref(false),backupEncryptionKeySaving=ref(false),activeTab=ref('basic');
const backupEncryptionPassphrase=ref('');
type LLMSettings={provider:string;baseUrl:string;model:string;timeoutSeconds:number;tokenLimitParameter:string;enabled:boolean;apiKeyConfigured:boolean;supportPrompt:string};
const llmLoading=ref(false),llmSaving=ref(false),llmKey=ref('');
const llmForm=reactive({provider:'openai',baseUrl:'https://api.openai.com/v1',model:'gpt-4o-mini',timeoutSeconds:30,tokenLimitParameter:'auto',enabled:false,apiKeyConfigured:false,supportPrompt:''});
async function loadLLM(){llmLoading.value=true;try{const data=await requestClient.get<LLMSettings>('/settings/ai/llm');Object.assign(llmForm,{provider:data.provider||'openai',baseUrl:data.baseUrl||'https://api.openai.com/v1',model:data.model||'gpt-4o-mini',timeoutSeconds:data.timeoutSeconds||30,tokenLimitParameter:data.tokenLimitParameter||'auto',enabled:!!data.enabled,apiKeyConfigured:!!data.apiKeyConfigured,supportPrompt:data.supportPrompt||''});llmKey.value='';}catch{message.error('AI 模型配置加载失败');}finally{llmLoading.value=false;}}
async function saveLLM(){if(!llmForm.provider.trim()||!llmForm.baseUrl.trim()||!llmForm.model.trim()){message.warning('请填写供应商、API Base URL 和模型名称');return;}llmSaving.value=true;const payload={...llmForm,apiKey:llmKey.value};try{await withAdminActionConfirmation({actionKey:'settings.ai.llm.update',method:'PUT',path:'/api/settings/ai/llm',payload,run:(token)=>requestClient.put<LLMSettings>('/settings/ai/llm',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});llmKey.value='';await loadLLM();message.success('AI 模型配置已安全保存');}catch(error:any){message.error(error?.response?.data?.message||'保存 AI 模型配置失败');}finally{llmSaving.value=false;}}
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const form=reactive<SiteSettings>({siteName:'TG-Card',siteLogo:'',contactEmail:'',smtpHost:'',smtpPort:587,smtpUsername:'',smtpPassword:'',smtpFromName:'TG-Card',smtpFromEmail:'',smtpUseTLS:true,emailOrderCreated:false,emailPaymentSuccess:true,emailDeliverySuccess:true,emailSupportCreated:true,telegramAdminChatId:'',telegramGroupChatId:'',telegramNotifyOrderCreated:true,telegramNotifyPaymentSuccess:true,telegramNotifyDeliverySuccess:true,telegramNotifySupportCreated:true,telegramNotifyStockLow:true,telegramNotifyDailyReport:true,telegramNotifyRiskAlert:true,stockAutoUnlistOnZero:true,stockAutoRelistOnRestock:false,capEnabled:false,capDomain:'',capSiteKey:'',capSecret:'',backupEnabled:true,backupTime:'03:00'});
async function load(){loading.value=true;try{const data=await requestClient.get<SiteSettings & {smtpPasswordConfigured:boolean;capSecretConfigured:boolean;backupEncryptionPassphraseConfigured:boolean}>('/settings/site');Object.assign(form,data);passwordConfigured.value=!!data.smtpPasswordConfigured;capSecretConfigured.value=!!data.capSecretConfigured;backupEncryptionKeyConfigured.value=!!data.backupEncryptionPassphraseConfigured;form.smtpPassword='';form.capSecret='';backupEncryptionPassphrase.value='';}catch{message.error('网站设置加载失败');}finally{loading.value=false;}}
async function save(){saving.value=true;try{const payload={...form};const data=await withAdminActionConfirmation({actionKey:'settings.site.update',method:'PUT',path:'/api/settings/site',payload,run:(token)=>requestClient.put<SiteSettings & {smtpPasswordConfigured:boolean;capSecretConfigured:boolean}>('/settings/site',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});Object.assign(form,data);form.smtpPassword='';passwordConfigured.value=!!data.smtpPasswordConfigured;capSecretConfigured.value=!!data.capSecretConfigured;form.capSecret='';message.success('网站设置已保存');}catch(error:any){message.error(error?.response?.data?.message||'保存网站设置失败');}finally{saving.value=false;}}
async function uploadSiteLogo(file:any){const image=file?.originFileObj||file;if(!(image instanceof Blob)){message.error('无法读取图标文件');return false;}if(image.size>20*1024*1024){message.warning('网站图标不能超过 20 MB');return false;}logoUploading.value=true;try{const body=new FormData();body.append('file',image,image.name||'site-logo');const result=await requestClient.post<{siteLogo:string}>('/settings/site/logo',body,{headers:{'Content-Type':'multipart/form-data'}});form.siteLogo=result.siteLogo;message.success('网站图标上传成功，请保存设置');}catch(error:any){message.error(error?.response?.data?.message||'网站图标上传失败');}finally{logoUploading.value=false;}return false;}
function generateBackupEncryptionKey(){
 try{
  const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
  const bytes=crypto.getRandomValues(new Uint8Array(32));
  backupEncryptionPassphrase.value=Array.from(bytes,(byte)=>alphabet[byte&63]).join('');
  message.success('已生成 32 位随机备份密钥，请保存后妥善保管');
 }catch{message.error('当前浏览器无法安全生成随机密钥');}
}
async function saveBackupEncryptionKey(){
 if(backupEncryptionPassphrase.value.length<32){message.warning('备份加密密钥至少需要 32 个字符');return;}
 backupEncryptionKeySaving.value=true;
 const payload={passphrase:backupEncryptionPassphrase.value};
 try{
  await withAdminActionConfirmation({actionKey:'settings.site.backup_encryption.update',method:'PUT',path:'/api/settings/site/backup-encryption-key',payload,run:(token)=>requestClient.put('/settings/site/backup-encryption-key',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});
  backupEncryptionKeyConfigured.value=true;backupEncryptionPassphrase.value='';
  message.success('数据库备份加密密钥已保存');
 }catch(error:any){message.error(error?.response?.data?.message||'保存备份加密密钥失败');}
 finally{backupEncryptionKeySaving.value=false;}
}
async function testEmail(){testing.value=true;try{await requestClient.post('/settings/site/test-email',{});message.success('测试邮件已发送，请检查收件箱');}catch(error:any){message.error(error?.response?.data?.message||'测试邮件发送失败');}finally{testing.value=false;}}
async function testTelegram(){testing.value=true;try{await requestClient.post('/settings/site/test-telegram',{});message.success('Telegram 测试通知已发送');}catch(error:any){message.error(error?.response?.data?.message||'Telegram 测试通知失败');}finally{testing.value=false;}}
onMounted(()=>{load();loadLLM();});
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div><h1 class="m-0 text-xl font-semibold">网站设置</h1><p class="mb-0 mt-1 text-xs leading-5 text-gray-500">按类别分开管理。配置会保存到数据库，无需修改服务器环境变量。</p></div>
  <a-card :bordered="false" :loading="loading" :body-style="{padding:'16px'}">
   <a-tabs v-model:activeKey="activeTab" :tab-position="isMobile?'top':'left'" :tab-bar-gutter="12" class="site-settings-tabs">
    <a-tab-pane key="basic">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:web" />基础信息</span></template>
     <div class="max-w-3xl">
      <div class="mb-4 text-sm font-semibold">站点基础信息</div>
      <a-form layout="vertical"><a-row :gutter="16">
       <a-col :xs="24" :md="12"><a-form-item label="站点名称"><a-input v-model:value="form.siteName" maxlength="100" /></a-form-item></a-col>
<a-col :span="24"><a-form-item label="网站图标"><div class="flex flex-wrap items-center gap-3"><img v-if="form.siteLogo" :src="form.siteLogo" alt="网站图标预览" class="h-14 w-14 rounded-lg border border-gray-200 object-contain p-1" /><a-upload accept="image/jpeg,image/png,image/webp,image/gif,image/bmp,image/tiff,image/avif,image/heic,image/heif" :show-upload-list="false" :before-upload="uploadSiteLogo"><a-button :loading="logoUploading">上传网站图标</a-button></a-upload><span class="text-xs text-gray-500">支持常见图片格式，最大 20 MB</span></div></a-form-item></a-col>
      </a-row></a-form>
     </div>
    </a-tab-pane>
    <a-tab-pane key="smtp">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:email-outline" />邮件服务</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">SMTP 邮件服务</div>
      
      <a-form layout="vertical"><a-row :gutter="16">
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 主机"><a-input v-model:value="form.smtpHost" placeholder="smtp.example.com" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 端口"><a-input-number v-model:value="form.smtpPort" class="w-full" :min="1" :max="65535" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 用户名"><a-input v-model:value="form.smtpUsername" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="SMTP 密码"><a-input-password v-model:value="form.smtpPassword" autocomplete="new-password" :placeholder="passwordConfigured?'已配置，留空保持不变':'请输入 SMTP 密码'" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="发件人名称"><a-input v-model:value="form.smtpFromName" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="发件人邮箱"><a-input v-model:value="form.smtpFromEmail" type="email" /></a-form-item></a-col>
      </a-row><a-form-item label="SMTP TLS"><a-switch v-model:checked="form.smtpUseTLS" /></a-form-item>
      </a-form>
      <a-form-item label="邮件通知收件邮箱（用于测试邮件及业务邮件通知）"><a-input v-model:value="form.contactEmail" type="email" placeholder="可选，填写接收通知的邮箱" /></a-form-item>
      <a-space wrap class="mt-3 gap-3"><a-button size="small" :loading="testing" :disabled="!form.contactEmail" @click="testEmail">发送测试邮件</a-button></a-space>
     </div>
    </a-tab-pane>
    <a-tab-pane key="email">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:bell-outline" />邮件通知</span></template>
     <div class="max-w-3xl">
      <div class="mb-4 text-sm font-semibold">邮件通知开关</div>
      
      <a-row :gutter="[16,12]">
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">新订单通知</span><a-switch v-model:checked="form.emailOrderCreated" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">付款成功通知</span><a-switch v-model:checked="form.emailPaymentSuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">自动发货通知</span><a-switch v-model:checked="form.emailDeliverySuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">售后工单通知</span><a-switch v-model:checked="form.emailSupportCreated" /></div></a-col>
      </a-row>
     </div>
    </a-tab-pane>
    <a-tab-pane key="inventory">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:package-variant-closed-check" />库存自动化</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">库存归零与补货上架策略</div>
      
      <div class="mb-3 flex items-center justify-between rounded-lg border border-gray-100 p-3">
       <div class="pr-4"><div class="text-sm font-medium">库存归零自动下架</div><div class="mt-1 text-xs text-gray-500">可售卡密库存为 0 时自动将商品下架，并通知 Owner。</div></div>
       <a-switch v-model:checked="form.stockAutoUnlistOnZero" />
      </div>
      <div class="flex items-center justify-between rounded-lg border border-gray-100 p-3">
       <div class="pr-4"><div class="text-sm font-medium">补货后自动重新上架</div><div class="mt-1 text-xs text-gray-500">开启后，已下架商品补充库存时自动上架；关闭时需在商品管理中手动上架。</div></div>
       <a-switch v-model:checked="form.stockAutoRelistOnRestock" />
      </div>
     </div>
    </a-tab-pane>
    <a-tab-pane key="telegram">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:telegram" />Telegram 通知</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">Telegram 私聊与群组通知</div>
      
      <a-form layout="vertical"><a-row :gutter="16">
        <a-col :xs="24" :md="12"><a-form-item label="管理员通知 Chat ID"><a-input v-model:value="form.telegramAdminChatId" /></a-form-item></a-col>
       <a-col :xs="24" :md="12"><a-form-item label="通知群组 Chat ID"><a-input v-model:value="form.telegramGroupChatId" /></a-form-item></a-col>
      </a-row></a-form>
      <div class="mb-3 mt-2 text-sm font-semibold">通知事件</div>
      <a-row :gutter="[16,12]">
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">新订单</span><a-switch v-model:checked="form.telegramNotifyOrderCreated" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">付款成功</span><a-switch v-model:checked="form.telegramNotifyPaymentSuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">发货成功</span><a-switch v-model:checked="form.telegramNotifyDeliverySuccess" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">售后工单</span><a-switch v-model:checked="form.telegramNotifySupportCreated" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">库存不足提醒（发送给主人）</span><a-switch v-model:checked="form.telegramNotifyStockLow" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">每日经营报告（发送给主人）</span><a-switch v-model:checked="form.telegramNotifyDailyReport" /></div></a-col>
       <a-col :xs="24" :sm="12"><div class="flex items-center justify-between rounded-lg border border-gray-100 p-3"><span class="text-sm">支付风险提醒（发送给主人）</span><a-switch v-model:checked="form.telegramNotifyRiskAlert" /></div></a-col>
      </a-row>
      <a-button class="mt-4" size="small" :loading="testing" @click="testTelegram">发送 Telegram 测试通知</a-button>
     </div>
    </a-tab-pane>
    <a-tab-pane key="ai">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:robot-outline" />AI 售后客服</span></template>
     <div class="max-w-3xl">
      <div class="mb-2 text-sm font-semibold">大模型服务配置</div>
      <p class="mb-4 text-xs leading-5 text-gray-500">支持 OpenAI、DeepSeek、Kimi / Moonshot（国际版与中国大陆版）、智谱 GLM 及自定义 OpenAI 兼容 Chat Completions 接口。请按 API 账号所属平台选择 Kimi 区域。API Key 使用服务端密钥加密保存；读取配置不会返回密钥。保存操作需要管理员二次确认。</p>
      <a-spin :spinning="llmLoading">
       <a-form layout="vertical">
        <a-form-item label="模型供应商"><a-select v-model:value="llmForm.provider"><a-select-option value="openai">OpenAI</a-select-option><a-select-option value="deepseek">DeepSeek</a-select-option><a-select-option value="kimi">Kimi / Moonshot 国际版</a-select-option><a-select-option value="moonshot-cn">Kimi / Moonshot 中国大陆版</a-select-option><a-select-option value="zhipu">智谱 GLM</a-select-option><a-select-option value="custom">自定义 OpenAI 兼容服务</a-select-option></a-select></a-form-item>
        <a-form-item label="API Base URL"><a-input v-model:value="llmForm.baseUrl" placeholder="https://api.openai.com/v1" /></a-form-item>
        <a-form-item label="模型名称"><a-input v-model:value="llmForm.model" placeholder="例如 gpt-4o-mini / deepseek-chat" /></a-form-item>
        <a-form-item v-if="llmForm.provider==='openai'" label="OpenAI Token 参数"><a-select v-model:value="llmForm.tokenLimitParameter"><a-select-option value="auto">自动（max_completion_tokens，默认）</a-select-option><a-select-option value="max_completion_tokens">max_completion_tokens（新接口）</a-select-option><a-select-option value="max_tokens">max_tokens（旧模型 / 兼容网关）</a-select-option></a-select><div class="mt-1 text-xs text-gray-500">不会根据模型名称猜测参数；请按当前模型和 API 网关要求明确选择。</div></a-form-item>
        <a-form-item label="API Key"><a-input-password v-model:value="llmKey" autocomplete="new-password" :placeholder="llmForm.apiKeyConfigured?'已加密保存，留空保持原密钥':'请输入模型 API Key'" /></a-form-item>
        <a-form-item label="请求超时（秒）"><a-input-number v-model:value="llmForm.timeoutSeconds" class="w-full" :min="1" :max="90" /></a-form-item>
        <a-form-item label="启用 AI 模型"><a-switch v-model:checked="llmForm.enabled" /></a-form-item>\n        <a-form-item label="AI 售后客服规则 / Prompt"><a-textarea v-model:value="llmForm.supportPrompt" :rows="6" :maxlength="3000" show-count placeholder="填写商品答疑范围、表达风格、常见问题处理指引。不要要求 AI 执行退款、补发或修改订单等操作。" /><div class="mt-1 text-xs text-gray-500">最多 3000 字；该规则作为模型指令附加在系统安全规则之后，不能覆盖退款、资金和人工转接限制。</div></a-form-item>
       </a-form>
       <a-space wrap>
        <a-button size="small" :loading="llmLoading" @click="loadLLM">重新加载</a-button>
        <a-button size="small" type="primary" :loading="llmSaving" @click="saveLLM">保存 AI 配置</a-button>
        <a-tag v-if="llmForm.apiKeyConfigured" color="success">API Key 已配置</a-tag>
        <a-tag v-else color="warning">API Key 未配置</a-tag>
       </a-space>
      </a-spin>
      <a-alert class="mt-4" type="warning" show-icon message="AI 已接入新建工单首条文字及后续纯文字消息。图片、文件仍由人工处理；退款、支付争议、人工请求、上下文读取失败或模型异常会转交人工。请先配置模型、验证 API Key 和客服规则，再启用。" />
     </div>
    </a-tab-pane>
    <a-tab-pane key="backup">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:database-clock-outline" />自动备份</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">每日数据库自动备份</div>
      
      <div class="mb-4 flex items-center justify-between rounded-lg border border-gray-100 p-3">
       <div class="pr-4"><div class="text-sm font-medium">启用每日自动备份</div><div class="mt-1 text-xs text-gray-500">关闭后不再自动创建或发送备份；现有手动备份/恢复脚本不受影响。</div></div>
       <a-switch v-model:checked="form.backupEnabled" />
      </div>
      <a-form layout="vertical">
       <a-form-item label="每日备份时间"><a-time-picker v-model:value="form.backupTime" format="HH:mm" value-format="HH:mm" class="w-full" :minute-step="5" /></a-form-item>
      </a-form>
      <a-divider />
      <div class="mb-2 text-sm font-semibold">数据库备份加密密钥</div>
      <a-form layout="vertical">
       <a-form-item label="32 位备份加密密钥">
        <a-input-password v-model:value="backupEncryptionPassphrase" autocomplete="new-password" :maxlength="128" placeholder="请输入密钥，或点击生成密钥" />
       </a-form-item>
      </a-form>
      <a-space wrap>
       <a-button size="small" @click="generateBackupEncryptionKey">生成密钥</a-button>
       <a-button size="small" type="primary" :loading="backupEncryptionKeySaving" :disabled="backupEncryptionPassphrase.length<32" @click="saveBackupEncryptionKey">保存密钥</a-button>
       <a-tag v-if="backupEncryptionKeyConfigured" color="success">已配置</a-tag>
       <a-tag v-else color="warning">未配置</a-tag>
      </a-space>
      <a-alert class="mt-4" type="warning" show-icon message="密钥至少 32 个字符。请在保存后立即复制并存入密码管理器或离线安全位置；丢失密钥将无法解密备份。更换密钥只影响之后生成的备份，旧备份仍必须使用原密钥恢复。" />
      <div class="mt-4 text-xs leading-6 text-gray-500">网站保存的密钥用于每日自动备份。服务器命令行手动备份/恢复仍需在运行命令的环境中提供 BACKUP_ENCRYPTION_PASSPHRASE。</div>
      <div class="mt-3 text-xs leading-6 text-gray-500">备份发送失败时会记录服务端错误日志，并尝试向主人发送失败通知。备份文件通过 Telegram 文档消息发送。</div>
     </div>
    </a-tab-pane>
    <a-tab-pane key="cap">
     <template #tab><span class="inline-flex items-center gap-2"><IconifyIcon icon="mdi:shield-check-outline" />人机验证</span></template>
     <div class="max-w-3xl">
      <div class="mb-3 text-sm font-semibold">Cap 人机验证</div>
      
      <div class="mb-4 flex items-center justify-between rounded-lg border border-gray-100 p-3">
       <div class="pr-4"><div class="text-sm font-medium">启用登录人机验证</div></div>
       <a-switch v-model:checked="form.capEnabled" />
      </div>
      <a-form layout="vertical">
       <a-form-item label="服务域名"><a-input v-model:value="form.capDomain" placeholder="https://cap.example.com" /></a-form-item>
       <a-form-item label="Site Key"><a-input v-model:value="form.capSiteKey" autocomplete="off" /></a-form-item>
       <a-form-item label="Secret Key">
        <a-input-password v-model:value="form.capSecret" autocomplete="new-password" :placeholder="capSecretConfigured?'已加密保存，留空保持不变':'请输入 Cap Secret Key'" />
        
       </a-form-item>
      </a-form>
      
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