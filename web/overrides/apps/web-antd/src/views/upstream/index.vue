<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';

interface UpstreamSettings {
  provider: string;
  baseUrl: string;
  merchantIdMasked: string;
  merchantIdConfigured: boolean;
  merchantSecretConfigured: boolean;
  enabled: boolean;
  catalogStale: boolean;
  markupPercent: number;
  markupFixedMinor: number;
  syncIntervalMinutes: number;
  syncBatchSize: number;
  syncBatchDelayMs: number;
  procurementPollIntervalSeconds: number;
  procurementTimeoutMinutes: number;
  supplierLowBalanceThreshold: string;
  callbackUrl: string;
  exchangeRateToUSDT: number;
  lastTestStatus: string;
  lastTestAt: string;
  lastError: string;
  syncStatus: string;
  syncAt: string;
  syncAttemptAt: string;
  syncError: string;
  updatedAt: string;
}
const loading=ref(false),saving=ref(false),testing=ref(false),syncing=ref(false);
const merchantId=ref(''),merchantSecret=ref(''),clearCredentials=ref(false);
const settings=reactive<UpstreamSettings>({
 provider:'dujiao_next',baseUrl:'',merchantIdMasked:'',merchantIdConfigured:false,
 merchantSecretConfigured:false,enabled:false,catalogStale:true,markupPercent:0,markupFixedMinor:0,syncIntervalMinutes:15,syncBatchSize:25,syncBatchDelayMs:1000,procurementPollIntervalSeconds:30,procurementTimeoutMinutes:30,supplierLowBalanceThreshold:'0',callbackUrl:'',exchangeRateToUSDT:0,lastTestStatus:'not_tested',lastTestAt:'',
 lastError:'',syncStatus:'not_started',syncAt:'',syncAttemptAt:'',syncError:'',updatedAt:'',
});
const testResult=ref<{siteName:string;protocolVersion:string;balance:string;currency:string}|null>(null);
interface ProcurementItem { localOrderId:string; localOrderNo:string; externalOrderNo:string; status:string; requestAttempts:number; lastError:string; updatedAt:string; }
const procurements=ref<ProcurementItem[]>([]);
const procurementLoading=ref(false),refundOrderId=ref(''),refundReason=ref(''),refundVisible=ref(false),actionLoading=ref('');
const resolveOrderId=ref(''),resolveVisible=ref(false),resolveResolution=ref<'accepted'|'not_accepted'>('not_accepted');
const resolveExternalOrderID=ref(''),resolveExternalOrderNo=ref(''),resolveNote=ref('');
const procurementStatus=(value:string)=>{const key='tg-card.upstream.procurementStatus.'+value;const text=$t(key);return text===key?value:text};
async function loadProcurements(){
 procurementLoading.value=true;
 try{procurements.value=await requestClient.get<ProcurementItem[]>('/upstream/procurements')||[];}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.queueLoad'));}
 finally{procurementLoading.value=false;}
}
async function retryProcurement(item:ProcurementItem){
 actionLoading.value=item.localOrderId;
 try{const path='/api/upstream/procurements/'+item.localOrderId+'/retry';const payload={};await withAdminActionConfirmation({actionKey:'upstream.procurement.retry',method:'POST',path,payload,run:(token)=>requestClient.post('/upstream/procurements/'+item.localOrderId+'/retry',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success($t('tg-card.upstream.messages.retryQueued'));await loadProcurements();}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.retry'));}
 finally{actionLoading.value='';}
}
function openRefund(item:ProcurementItem){refundOrderId.value=item.localOrderId;refundReason.value='';refundVisible.value=true;}
async function submitRefund(){
 if(!refundReason.value.trim()){message.warning($t('tg-card.upstream.messages.refundReason'));return;}
 actionLoading.value=refundOrderId.value;
 try{const path='/api/upstream/procurements/'+refundOrderId.value+'/refund';const payload={reason:refundReason.value.trim()};await withAdminActionConfirmation({actionKey:'upstream.procurement.refund',method:'POST',path,payload,run:(token)=>requestClient.post('/upstream/procurements/'+refundOrderId.value+'/refund',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});message.success($t('tg-card.upstream.messages.refundCredited'));refundVisible.value=false;await loadProcurements();}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.refund'));}
 finally{actionLoading.value='';}
}
function openResolve(item:ProcurementItem){
 resolveOrderId.value=item.localOrderId;resolveResolution.value='not_accepted';
 resolveExternalOrderID.value='';resolveExternalOrderNo.value='';resolveNote.value='';resolveVisible.value=true;
}
async function submitResolve(){
 if(!resolveNote.value.trim()){message.warning($t('tg-card.upstream.messages.supplierEvidence'));return;}
 if(resolveResolution.value==='accepted'&&!/^\d+$/.test(resolveExternalOrderID.value.trim())){message.warning($t('tg-card.upstream.messages.externalOrderId'));return;}
 actionLoading.value=resolveOrderId.value;
 try{
  const path='/api/upstream/procurements/'+resolveOrderId.value+'/resolve';
  const payload={resolution:resolveResolution.value,externalOrderId:resolveExternalOrderID.value.trim(),externalOrderNo:resolveExternalOrderNo.value.trim(),note:resolveNote.value.trim()};
  await withAdminActionConfirmation({actionKey:'upstream.procurement.resolve',method:'POST',path,payload,run:(token)=>requestClient.post('/upstream/procurements/'+resolveOrderId.value+'/resolve',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});
  message.success(resolveResolution.value==='accepted'?$t('tg-card.upstream.messages.accepted'):$t('tg-card.upstream.messages.notAccepted'));
  resolveVisible.value=false;await loadProcurements();
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.resolve'));}
 finally{actionLoading.value='';}
}
const syncResult=ref<{categories:number;products:number;imagesFailed:number;warning:string}|null>(null);
const testLabel=computed(()=>{const key='tg-card.upstream.testStatus.'+settings.lastTestStatus;const value=$t(key);return value===key?settings.lastTestStatus:value});
const syncLabel=computed(()=>{const key='tg-card.upstream.syncStatus.'+settings.syncStatus;const value=$t(key);return value===key?settings.syncStatus:value});
async function load(){
 loading.value=true;
 try{const data=await requestClient.get<UpstreamSettings>('/upstream/config');Object.assign(settings,data);}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.settingsLoad'));}
 finally{loading.value=false;void loadProcurements();}
}
async function save():Promise<boolean>{
 saving.value=true;
 try{
  const requestedEnabled=settings.enabled;
  const path='/api/upstream/config';
  const payload={
   provider:settings.provider,baseUrl:settings.baseUrl,merchantId:merchantId.value,
   merchantSecret:merchantSecret.value,clearCredentials:clearCredentials.value,enabled:settings.enabled,
   markupPercent:settings.markupPercent,markupFixedMinor:settings.markupFixedMinor,syncIntervalMinutes:settings.syncIntervalMinutes,syncBatchSize:settings.syncBatchSize,syncBatchDelayMs:settings.syncBatchDelayMs,procurementPollIntervalSeconds:settings.procurementPollIntervalSeconds,procurementTimeoutMinutes:settings.procurementTimeoutMinutes,supplierLowBalanceThreshold:settings.supplierLowBalanceThreshold,callbackUrl:settings.callbackUrl,exchangeRateToUSDT:settings.exchangeRateToUSDT,
  };
  const data=await withAdminActionConfirmation({actionKey:'settings.upstream.update',method:'PUT',path,payload,run:(token)=>requestClient.put<UpstreamSettings>('/upstream/config',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});
  Object.assign(settings,data);merchantId.value='';merchantSecret.value='';clearCredentials.value=false;testResult.value=null;
  if(requestedEnabled&&!settings.enabled){message.info($t('tg-card.upstream.messages.savedDisabled'));}else{message.success($t('tg-card.upstream.messages.saved'));}return true;
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.settingsSave'));return false;}
 finally{saving.value=false;}
}
async function syncNow(){
 if(clearCredentials.value){message.warning($t('tg-card.upstream.messages.saveBeforeSync'));return;}
 if(merchantId.value||merchantSecret.value||!settings.merchantIdConfigured||!settings.merchantSecretConfigured){const saved=await save();if(!saved)return;}
 syncing.value=true;syncResult.value=null;syncProgress.status='queued';
 try{
  const started=await requestClient.post<any>('/upstream/sync',{});
  syncJobId.value=started.jobId;
  if(!syncJobId.value)throw new Error($t('tg-card.upstream.errors.taskId'));
  while(syncing.value){
   const state=await requestClient.get<any>('/upstream/sync/'+syncJobId.value);
   Object.assign(syncProgress,state);
   if(['success','failed','cancelled','paused'].includes(state.status)){
    if(state.status==='success'){await load();message.success($t('tg-card.upstream.messages.syncDone'));}
    else if(state.status==='cancelled'){await load();message.info($t('tg-card.upstream.messages.syncCancelled'));}
    else{await load();message.error(state.lastError||$t('tg-card.upstream.errors.sync'));}
    break;
   }
   await new Promise(resolve=>setTimeout(resolve,1500));
  }
 }catch(error:any){await load();message.error(error?.response?.data?.message||error?.message||$t('tg-card.upstream.errors.syncStart'));}
 finally{syncing.value=false;syncJobId.value='';}
}
async function disableUpstream(){
 syncing.value=true;syncProgress.status='cleaning';syncResult.value=null;
 try{
  const path='/api/upstream/disable';const payload={};
  const started=await withAdminActionConfirmation({actionKey:'settings.upstream.disable',method:'POST',path,payload,run:(token)=>requestClient.post<any>('/upstream/disable',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)});
  syncJobId.value=started.jobId;
  if(!syncJobId.value)throw new Error($t('tg-card.upstream.errors.cleanupTaskId'));
  while(syncing.value){
   const state=await requestClient.get<any>('/upstream/sync/'+syncJobId.value);
   Object.assign(syncProgress,state);
   if(['success','failed','paused'].includes(state.status)){
    await load();
    if(state.status==='success')message.success($t('tg-card.upstream.messages.dataCleared'));
    else message.error(state.lastError||$t('tg-card.upstream.errors.cleanup'));
    break;
   }
   await new Promise(resolve=>setTimeout(resolve,1500));
  }
 }catch(error:any){await load();message.error(error?.response?.data?.message||error?.message||$t('tg-card.upstream.errors.cleanupStart'));}
 finally{syncing.value=false;syncJobId.value='';}
}
async function cancelSync(){
 if(!syncJobId.value)return;
 try{await requestClient.post('/upstream/sync/'+syncJobId.value+'/cancel',{});message.info($t('tg-card.upstream.messages.cancelRequested'));}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.cancel'));}
}
async function testConnection(){
 if(clearCredentials.value){message.warning($t('tg-card.upstream.messages.saveBeforeTest'));return;}
 if(merchantId.value||merchantSecret.value||!settings.merchantIdConfigured||!settings.merchantSecretConfigured){const saved=await save();if(!saved)return;}
 testing.value=true;testResult.value=null;
 try{
  const data=await requestClient.post<any>('/upstream/test',{});
  testResult.value={siteName:data.siteName||'',protocolVersion:data.protocolVersion||'',balance:data.balance||'',currency:data.currency||''};
  await load();message.success($t('tg-card.upstream.messages.testSuccess'));
 }catch(error:any){await load();message.error(error?.response?.data?.message||$t('tg-card.upstream.errors.test'));}
 finally{testing.value=false;}
}
async function resumeActiveSyncJob(){
 try{
  const active=await requestClient.get<any>('/upstream/sync/active');
  if(!active?.id)return;
  syncJobId.value=active.id;Object.assign(syncProgress,active);syncing.value=true;
  while(syncing.value){
   const state=await requestClient.get<any>('/upstream/sync/'+syncJobId.value);
   Object.assign(syncProgress,state);
   if(['success','failed','cancelled','paused'].includes(state.status)){
    await load();
    if(state.status==='success'){
     if(state.triggerType==='manual_disable')message.success($t('tg-card.upstream.messages.dataCleared'));
     else message.success($t('tg-card.upstream.messages.syncDone'));
    }else if(state.status==='cancelled')message.info($t('tg-card.upstream.messages.syncCancelledKept'));
    else message.error(state.lastError||$t('tg-card.upstream.messages.taskFailed'));
    break;
   }
   await new Promise(resolve=>setTimeout(resolve,1500));
  }
 }catch(error:any){message.warning(error?.response?.data?.message||$t('tg-card.upstream.errors.runningTask'));}
 finally{syncing.value=false;syncJobId.value='';}
}

onMounted(()=>{void load();void resumeActiveSyncJob();});
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <a-card :bordered="false" :loading="loading" :title="$t('tg-card.upstream.title')">
   <template #extra><a-tag :color="settings.enabled&&!settings.catalogStale?'success':'default'">{{settings.enabled&&!settings.catalogStale?$t('tg-card.upstream.catalogSelling'):$t('tg-card.upstream.localSelling')}}</a-tag></template>
   
   <a-form layout="vertical">
    <a-form-item :label="$t('tg-card.upstream.supplierType')">
     <a-select v-model:value="settings.provider" disabled>
      <a-select-option value="dujiao_next">{{$t('tg-card.upstream.supplierType')}}</a-select-option>
     </a-select>
    </a-form-item>
    <a-form-item :label="$t('tg-card.upstream.siteAddress')" :extra="$t('tg-card.upstream.siteAddressExtra')">
     <a-input v-model:value="settings.baseUrl" placeholder="https://supplier.example.com" autocomplete="url"/>
    </a-form-item>
    <a-form-item :label="$t('tg-card.upstream.merchantKey')" :extra="settings.merchantIdConfigured?$t('tg-card.upstream.merchantKeySaved',{masked:settings.merchantIdMasked}):$t('tg-card.upstream.merchantKeyExtra')">
     <a-input v-model:value="merchantId" autocomplete="off" :placeholder="$t('tg-card.upstream.merchantKeyPlaceholder')"/>
    </a-form-item>
    <a-form-item :label="$t('tg-card.upstream.merchantSecret')" :extra="settings.merchantSecretConfigured?$t('tg-card.upstream.secretSaved'):$t('tg-card.upstream.secretExtra')">
     <a-input-password v-model:value="merchantSecret" autocomplete="new-password" :placeholder="$t('tg-card.upstream.secretPlaceholder')"/>
    </a-form-item>
    <a-form-item v-if="settings.merchantIdConfigured||settings.merchantSecretConfigured" :label="$t('tg-card.upstream.credentials')">
     <a-checkbox v-model:checked="clearCredentials">{{$t('tg-card.upstream.clearCredentials')}}</a-checkbox>
    </a-form-item>
    <a-divider/>
    <a-form-item :label="$t('tg-card.upstream.markup')">
     <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <div><div class="mb-1 text-xs text-gray-500">{{$t('tg-card.upstream.markupPercent')}}</div><a-input-number v-model:value="settings.markupPercent" class="w-full" :min="0" :max="1000" :step="1" addon-after="%"/></div>
      <div><div class="mb-1 text-xs text-gray-500">{{$t('tg-card.upstream.markupFixed')}}</div><a-input-number v-model:value="settings.markupFixedMinor" class="w-full" :min="0" :max="9000000000000" :step="1"/></div>
     </div>
     <div class="mt-2 text-xs text-gray-500">{{$t('tg-card.upstream.pricingFormula')}}</div>
    </a-form-item>
    <a-form-item :label="$t('tg-card.upstream.exchangeRate')" extra="填写 1 单位上游币种可兑换多少 USDT。例如上游为 CNY 时可填 7.20；上游币种为 USDT 时自动按 1 计算。非 USDT 币种未配置汇率将拒绝同步，避免错误定价。">
     <a-input-number v-model:value="settings.exchangeRateToUSDT" class="w-full" :min="0" :max="1000000" :step="0.01" :placeholder="$t('tg-card.upstream.exchangeRatePlaceholder')"/>
    </a-form-item>
    <a-form-item :label="$t('tg-card.upstream.syncInterval')">
     <a-select v-model:value="settings.syncIntervalMinutes" class="w-full">
      <a-select-option :value="5">{{$t('tg-card.upstream.minutes5')}}</a-select-option><a-select-option :value="15">{{$t('tg-card.upstream.minutes15')}}</a-select-option><a-select-option :value="30">{{$t('tg-card.upstream.minutes30')}}</a-select-option><a-select-option :value="60">{{$t('tg-card.upstream.hour1')}}</a-select-option><a-select-option :value="360">{{$t('tg-card.upstream.hours6')}}</a-select-option><a-select-option :value="1440">{{$t('tg-card.upstream.daily')}}</a-select-option>
     </a-select>
    </a-form-item>
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
     <a-form-item :label="$t('tg-card.upstream.batchSize')" extra="每页处理 20–50 个上游商品；默认 25。"><a-input-number v-model:value="settings.syncBatchSize" class="w-full" :min="20" :max="50" :step="5"/></a-form-item>
     <a-form-item :label="$t('tg-card.upstream.batchDelay')" extra="每批提交后暂停 250–10000 毫秒；默认 1000。"><a-input-number v-model:value="settings.syncBatchDelayMs" class="w-full" :min="250" :max="10000" :step="250"/></a-form-item>
    </div>
    <a-form-item :label="$t('tg-card.upstream.enableSelling')">
     <div class="flex flex-wrap items-center gap-3">
      <a-switch v-model:checked="settings.enabled"  :disabled="settings.enabled||settings.catalogStale||settings.syncStatus!=='success'"/>
      <span class="text-sm text-gray-500">{{$t('tg-card.upstream.enableRules')}}</span>
     </div>
    </a-form-item>
    <a-divider />
    <a-form-item :label="$t('tg-card.upstream.callback')" extra="请填写本站公网 HTTPS 地址，路径固定为 /api/v1/upstream/callback。配置后上游交付完成会主动回调；轮询仍作为兜底。">
     <a-input v-model:value="settings.callbackUrl" :placeholder="$t('tg-card.upstream.callbackPlaceholder')" autocomplete="url"/>
    </a-form-item>
    <a-form-item :label="$t('tg-card.upstream.pollInterval')"><a-input-number v-model:value="settings.procurementPollIntervalSeconds" class="w-full" :min="5" :max="3600" :step="5"/></a-form-item>
    <a-form-item :label="$t('tg-card.upstream.timeout')"><a-input-number v-model:value="settings.procurementTimeoutMinutes" class="w-full" :min="1" :max="1440" :step="1"/></a-form-item>
    <a-form-item :label="$t('tg-card.upstream.lowBalance')" extra="按上游账户自身币种填写，0 表示关闭低余额阈值告警。"><a-input v-model:value="settings.supplierLowBalanceThreshold" :placeholder="$t('tg-card.upstream.lowBalancePlaceholder')"/></a-form-item>
    
    <div class="flex flex-wrap items-center gap-3">
     <a-button type="primary" :loading="saving" @click="save">{{$t('tg-card.upstream.save')}}</a-button>
     <a-button :loading="testing" :disabled="saving||testing||syncing||clearCredentials||(!settings.merchantIdConfigured&&!merchantId)||(!settings.merchantSecretConfigured&&!merchantSecret)" @click="testConnection">{{$t('tg-card.upstream.testConnection')}}</a-button>
     <a-button :loading="syncing" :disabled="saving||testing||syncing||clearCredentials||(!settings.merchantIdConfigured&&!merchantId)||(!settings.merchantSecretConfigured&&!merchantSecret)" @click="syncNow">{{$t('tg-card.upstream.syncNow')}}</a-button>
     <a-button v-if="syncing&&syncProgress.status!=='cleaning'" danger :disabled="!syncJobId" @click="cancelSync">{{$t('tg-card.upstream.cancelSync')}}</a-button>
     <a-popconfirm :title="$t('tg-card.upstream.disableConfirm')" :ok-text="$t('tg-card.upstream.confirmClear')" :cancel-text="$t('tg-card.upstream.cancel')" @confirm="disableUpstream"><a-button danger :disabled="!settings.enabled||syncing">{{$t('tg-card.upstream.disableTitle')}}</a-button></a-popconfirm>
     <a-button :disabled="saving||testing||syncing" @click="load">{{$t('tg-card.upstream.reload')}}</a-button>
    </div>
   </a-form>
  </a-card>
  <a-card :bordered="false" :title="$t('tg-card.upstream.procurementQueue')">
   <template #extra><a-button size="small" :loading="procurementLoading" @click="loadProcurements">{{$t('tg-card.upstream.refreshQueue')}}</a-button></template>
   <a-table :data-source="procurements" :loading="procurementLoading" :pagination="{pageSize:8}" row-key="localOrderId" :scroll="{x:760}">
    <a-table-column :title="$t('tg-card.upstream.localOrderNo')" data-index="localOrderNo" :width="190"/>
    <a-table-column :title="$t('tg-card.upstream.externalOrderNo')" data-index="externalOrderNo" :width="170"/>
    <a-table-column :title="$t('tg-card.upstream.status')" :width="130"><template #default="{record}"><a-tag :color="record.status==='failed'||record.status==='uncertain'?'error':record.status==='completed'?'success':'processing'">{{procurementStatus(record.status)}}</a-tag></template></a-table-column>
    <a-table-column :title="$t('tg-card.upstream.retryCount')" data-index="requestAttempts" :width="90"/>
    <a-table-column :title="$t('tg-card.upstream.reason')" data-index="lastError" :ellipsis="true" :width="260"/>
    <a-table-column :title="$t('tg-card.upstream.actions')" :fixed="'right'" :width="170"><template #default="{record}">
     <div v-if="record.status==='uncertain'">
      <a-button size="small" type="primary" :loading="actionLoading===record.localOrderId" @click="openResolve(record)">{{$t('tg-card.upstream.verifyAcceptance')}}</a-button>
     </div>
     <div class="flex gap-2" v-else-if="record.status==='failed'">
      <a-button size="small" type="primary" :loading="actionLoading===record.localOrderId" @click="retryProcurement(record)">{{record.externalOrderNo?$t('tg-card.upstream.queryRecovery'):$t('tg-card.upstream.manualRetry')}}</a-button>
      <a-button size="small" danger :disabled="!!actionLoading" @click="openRefund(record)">{{$t('tg-card.upstream.refund')}}</a-button>
     </div><span v-else class="text-xs text-gray-400">—</span>
    </template></a-table-column>
   </a-table>
   
  </a-card>
  <a-modal v-model:open="resolveVisible" :title="$t('tg-card.upstream.resolveTitle')" :confirm-loading="actionLoading===resolveOrderId" @ok="submitResolve">
   <a-alert class="mb-4" type="warning" show-icon :message="$t('tg-card.upstream.resolveNotice')" />
   <a-form-item :label="$t('tg-card.upstream.supplierResult')">
    <a-radio-group v-model:value="resolveResolution">
     <a-radio value="not_accepted">{{$t('tg-card.upstream.notAccepted')}}</a-radio>
     <a-radio value="accepted">{{$t('tg-card.upstream.accepted')}}</a-radio>
    </a-radio-group>
   </a-form-item>
   <template v-if="resolveResolution==='accepted'">
    <a-form-item :label="$t('tg-card.upstream.externalId')"><a-input v-model:value="resolveExternalOrderID" :placeholder="$t('tg-card.upstream.externalIdPlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.upstream.externalNo')"><a-input v-model:value="resolveExternalOrderNo" :placeholder="$t('tg-card.upstream.externalNoPlaceholder')"/></a-form-item>
   </template>
   <a-form-item :label="$t('tg-card.upstream.evidence')"><a-textarea v-model:value="resolveNote" :rows="3" :maxlength="500" show-count :placeholder="$t('tg-card.upstream.evidencePlaceholder')"/></a-form-item>
  </a-modal>
  <a-modal v-model:open="refundVisible" :title="$t('tg-card.upstream.refundTitle')" :confirm-loading="!!actionLoading" @ok="submitRefund">
   <a-alert type="warning" show-icon :message="$t('tg-card.upstream.refundNotice')" />
   <a-form-item class="mt-4" :label="$t('tg-card.upstream.refundReason')"><a-textarea v-model:value="refundReason" :rows="3" :maxlength="500" show-count :placeholder="$t('tg-card.upstream.refundReasonPlaceholder')"/></a-form-item>
  </a-modal>
  <a-card :bordered="false" :title="$t('tg-card.upstream.connectionStatus')">
   <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
    <div class="rounded-lg border border-gray-200 p-4">
     <div class="mb-2 text-sm text-gray-500">{{$t('tg-card.upstream.connectionTest')}}</div>
     <a-tag :color="settings.lastTestStatus==='success'?'success':settings.lastTestStatus==='failed'?'error':'default'">{{testLabel}}</a-tag>
     <div v-if="settings.lastTestAt" class="mt-2 text-xs text-gray-500">{{$t('tg-card.upstream.testTime')}}{{settings.lastTestAt}}</div>
     <a-alert v-if="settings.lastError" class="mt-3" type="error" show-icon :message="settings.lastError"/>
     <div v-if="testResult" class="mt-3 text-sm">{{$t('tg-card.upstream.site')}}{{testResult.siteName}} · {{$t('tg-card.upstream.protocol')}}{{testResult.protocolVersion}} · {{$t('tg-card.upstream.balance')}}{{testResult.balance}} {{testResult.currency}}</div>
    </div>
    <div class="rounded-lg border border-gray-200 p-4">
     <div class="mb-2 text-sm text-gray-500">{{$t('tg-card.upstream.productSync')}}</div>
     <a-tag color="default">{{syncLabel}}</a-tag>
     <div v-if="settings.syncAt" class="mt-2 text-xs text-gray-500">{{$t('tg-card.upstream.lastSync')}}{{settings.syncAt}}</div>
     <div v-if="settings.syncAttemptAt" class="mt-1 text-xs text-gray-500">{{$t('tg-card.upstream.lastAttempt')}}{{settings.syncAttemptAt}}</div>
     <div v-if="syncing" class="mt-2 text-xs text-gray-600">{{$t('tg-card.upstream.taskStatus')}}{{syncProgress.status}} · {{$t('tg-card.upstream.processed')}} {{syncProgress.processedItems}} / {{syncProgress.totalItems}} · {{$t('tg-card.upstream.currentPage')}} {{syncProgress.currentPage}} · {{$t('tg-card.upstream.errorsCount')}} {{syncProgress.errorCount}}</div>
     <a-alert v-if="syncProgress.lastError" class="mt-2" type="warning" show-icon :message="syncProgress.lastError"/>
     <a-alert v-if="settings.syncError" class="mt-3" type="warning" show-icon :message="settings.syncError"/>
     <div v-else class="mt-2 text-xs text-gray-500">{{$t('tg-card.upstream.autoSync')}}</div>
     <div v-if="syncResult" class="mt-3 text-sm">{{$t('tg-card.upstream.syncResult')}}{{syncResult.categories}}{{$t('tg-card.upstream.categories')}} · {{syncResult.products}}{{$t('tg-card.upstream.products')}} · {{$t('tg-card.upstream.imageFailures',{count:syncResult.imagesFailed})}}</div>
     <a-alert v-if="syncResult?.warning" class="mt-3" type="warning" show-icon :message="syncResult.warning"/>
    </div>
   </div>
  </a-card>
 </div>
</template>
