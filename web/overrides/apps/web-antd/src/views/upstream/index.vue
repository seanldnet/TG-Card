<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

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
const procurementStatus=(value:string)=>({queued:'待采购',polling:'查单中',completed:'已完成',failed:'采购失败',uncertain:'受理状态不确定',refunded:'已退款'} as Record<string,string>)[value]||value;
async function loadProcurements(){
 procurementLoading.value=true;
 try{procurements.value=await requestClient.get<ProcurementItem[]>('/upstream/procurements')||[];}
 catch(error:any){message.error(error?.response?.data?.message||'采购队列加载失败');}
 finally{procurementLoading.value=false;}
}
async function retryProcurement(item:ProcurementItem){
 actionLoading.value=item.localOrderId;
 try{await requestClient.post('/upstream/procurements/'+item.localOrderId+'/retry',{});message.success('已加入采购重试队列');await loadProcurements();}
 catch(error:any){message.error(error?.response?.data?.message||'采购重试失败');}
 finally{actionLoading.value='';}
}
function openRefund(item:ProcurementItem){refundOrderId.value=item.localOrderId;refundReason.value='';refundVisible.value=true;}
async function submitRefund(){
 if(!refundReason.value.trim()){message.warning('请填写退款原因');return;}
 actionLoading.value=refundOrderId.value;
 try{await requestClient.post('/upstream/procurements/'+refundOrderId.value+'/refund',{reason:refundReason.value.trim()});message.success('退款已记入买家站内余额');refundVisible.value=false;await loadProcurements();}
 catch(error:any){message.error(error?.response?.data?.message||'采购退款失败');}
 finally{actionLoading.value='';}
}
const syncResult=ref<{categories:number;products:number;imagesFailed:number;warning:string}|null>(null);
const testLabel=computed(()=>{const labels:Record<string,string>={success:'连接成功',failed:'连接失败',not_tested:'尚未测试'};return labels[settings.lastTestStatus]||settings.lastTestStatus;});
const syncLabel=computed(()=>{const labels:Record<string,string>={not_started:'尚未同步',idle:'空闲',running:'同步中',cleaning:'正在清理上游数据',success:'同步成功',failed:'同步失败',cancelled:'已取消',paused:'已暂停'};return labels[settings.syncStatus]||settings.syncStatus;});
async function load(){
 loading.value=true;
 try{const data=await requestClient.get<UpstreamSettings>('/upstream/config');Object.assign(settings,data);}
 catch(error:any){message.error(error?.response?.data?.message||'上游配置加载失败');}
 finally{loading.value=false;void loadProcurements();}
}
async function save():Promise<boolean>{
 saving.value=true;
 try{
  const requestedEnabled=settings.enabled;
  const data=await requestClient.put<UpstreamSettings>('/upstream/config',{
   provider:settings.provider,baseUrl:settings.baseUrl,merchantId:merchantId.value,
   merchantSecret:merchantSecret.value,clearCredentials:clearCredentials.value,enabled:settings.enabled,
   markupPercent:settings.markupPercent,markupFixedMinor:settings.markupFixedMinor,syncIntervalMinutes:settings.syncIntervalMinutes,syncBatchSize:settings.syncBatchSize,syncBatchDelayMs:settings.syncBatchDelayMs,procurementPollIntervalSeconds:settings.procurementPollIntervalSeconds,procurementTimeoutMinutes:settings.procurementTimeoutMinutes,supplierLowBalanceThreshold:settings.supplierLowBalanceThreshold,callbackUrl:settings.callbackUrl,exchangeRateToUSDT:settings.exchangeRateToUSDT,
  });
  Object.assign(settings,data);merchantId.value='';merchantSecret.value='';clearCredentials.value=false;testResult.value=null;
  if(requestedEnabled&&!settings.enabled){message.info('配置已保存；站点、凭证或价格策略有变化，上游展示已保持关闭，请重新同步后再启用。');}else{message.success('上游配置已安全保存');}return true;
 }catch(error:any){message.error(error?.response?.data?.message||'上游配置保存失败');return false;}
 finally{saving.value=false;}
}
async function syncNow(){
 if(clearCredentials.value){message.warning('请先保存配置后再同步');return;}
 if(merchantId.value||merchantSecret.value||!settings.merchantIdConfigured||!settings.merchantSecretConfigured){const saved=await save();if(!saved)return;}
 syncing.value=true;syncResult.value=null;syncProgress.status='queued';
 try{
  const started=await requestClient.post<any>('/upstream/sync',{});
  syncJobId.value=started.jobId;
  if(!syncJobId.value)throw new Error('同步任务未返回任务 ID');
  while(syncing.value){
   const state=await requestClient.get<any>('/upstream/sync/'+syncJobId.value);
   Object.assign(syncProgress,state);
   if(['success','failed','cancelled','paused'].includes(state.status)){
    if(state.status==='success'){await load();message.success('上游商品同步完成');}
    else if(state.status==='cancelled'){await load();message.info('同步已取消，已完成的数据批次予以保留');}
    else{await load();message.error(state.lastError||'上游商品同步失败');}
    break;
   }
   await new Promise(resolve=>setTimeout(resolve,1500));
  }
 }catch(error:any){await load();message.error(error?.response?.data?.message||error?.message||'上游商品同步启动或查询失败');}
 finally{syncing.value=false;syncJobId.value='';}
}
async function disableUpstream(){
 syncing.value=true;syncProgress.status='cleaning';syncResult.value=null;
 try{
  const started=await requestClient.post<any>('/upstream/disable',{});
  syncJobId.value=started.jobId;
  if(!syncJobId.value)throw new Error('清理任务未返回任务 ID');
  while(syncing.value){
   const state=await requestClient.get<any>('/upstream/sync/'+syncJobId.value);
   Object.assign(syncProgress,state);
   if(['success','failed','paused'].includes(state.status)){
    await load();
    if(state.status==='success')message.success('上游数据已清空，已恢复本地商品售卖');
    else message.error(state.lastError||'上游数据清理失败；可再次点击清理按钮重试');
    break;
   }
   await new Promise(resolve=>setTimeout(resolve,1500));
  }
 }catch(error:any){await load();message.error(error?.response?.data?.message||error?.message||'上游清理任务启动或查询失败');}
 finally{syncing.value=false;syncJobId.value='';}
}
async function cancelSync(){
 if(!syncJobId.value)return;
 try{await requestClient.post('/upstream/sync/'+syncJobId.value+'/cancel',{});message.info('已提交取消请求，正在等待当前批次安全结束');}
 catch(error:any){message.error(error?.response?.data?.message||'取消同步失败');}
}
async function testConnection(){
 if(clearCredentials.value){message.warning('请先保存配置后再测试连接，不能在清除凭证后测试');return;}
 if(merchantId.value||merchantSecret.value||!settings.merchantIdConfigured||!settings.merchantSecretConfigured){const saved=await save();if(!saved)return;}
 testing.value=true;testResult.value=null;
 try{
  const data=await requestClient.post<any>('/upstream/test',{});
  testResult.value={siteName:data.siteName||'',protocolVersion:data.protocolVersion||'',balance:data.balance||'',currency:data.currency||''};
  await load();message.success('Dujiao-Next 连接测试成功');
 }catch(error:any){await load();message.error(error?.response?.data?.message||'上游连接测试失败');}
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
     if(state.triggerType==='manual_disable')message.success('上游数据已清空，已恢复本地商品售卖');
     else message.success('上游商品同步完成');
    }else if(state.status==='cancelled')message.info('同步已取消，已完成批次予以保留');
    else message.error(state.lastError||'上游任务执行失败');
    break;
   }
   await new Promise(resolve=>setTimeout(resolve,1500));
  }
 }catch(error:any){message.warning(error?.response?.data?.message||'读取进行中的上游任务失败');}
 finally{syncing.value=false;syncJobId.value='';}
}

onMounted(()=>{void load();void resumeActiveSyncJob();});
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <a-card :bordered="false" :loading="loading" title="上游对接">
   <template #extra><a-tag :color="settings.enabled&&!settings.catalogStale?'success':'default'">{{settings.enabled&&!settings.catalogStale?'上游售卖中':'本地商品售卖'}}</a-tag></template>
   <a-alert class="mb-5" type="info" show-icon message="当前仅开放 Dujiao-Next 官方 Open API。异次元和萌次元尚无已核实的第三方商户采购协议，因此不提供可调用配置。" />
   <a-form layout="vertical">
    <a-form-item label="上游类型">
     <a-select v-model:value="settings.provider" disabled>
      <a-select-option value="dujiao_next">Dujiao-Next（官方 Open API）</a-select-option>
     </a-select>
    </a-form-item>
    <a-form-item label="上游站点地址" extra="填写上游站点 HTTPS 根地址，不要添加 /api/v1/upstream。">
     <a-input v-model:value="settings.baseUrl" placeholder="https://supplier.example.com" autocomplete="url"/>
    </a-form-item>
    <a-form-item label="商户 ID / API Key" :extra="settings.merchantIdConfigured?'已保存凭证：'+settings.merchantIdMasked+'；留空表示保持原值。':'对应 Dujiao-Next API Key。'">
     <a-input v-model:value="merchantId" autocomplete="off" placeholder="输入 Dujiao-Next API Key；留空保持已保存值"/>
    </a-form-item>
    <a-form-item label="商户密钥 / API Secret" :extra="settings.merchantSecretConfigured?'密钥已加密保存且不会回显；留空表示保持原密钥。':'对应 Dujiao-Next API Secret，将使用 AES-256-GCM 加密保存。'">
     <a-input-password v-model:value="merchantSecret" autocomplete="new-password" placeholder="输入 API Secret；留空保持已保存值"/>
    </a-form-item>
    <a-form-item v-if="settings.merchantIdConfigured||settings.merchantSecretConfigured" label="凭证管理">
     <a-checkbox v-model:checked="clearCredentials">清除已保存的 API Key 和 API Secret</a-checkbox>
    </a-form-item>
    <a-divider/>
    <a-form-item label="上游价格加价策略">
     <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <div><div class="mb-1 text-xs text-gray-500">百分比加价</div><a-input-number v-model:value="settings.markupPercent" class="w-full" :min="0" :max="1000" :step="1" addon-after="%"/></div>
      <div><div class="mb-1 text-xs text-gray-500">固定加价（USDT 分）</div><a-input-number v-model:value="settings.markupFixedMinor" class="w-full" :min="0" :max="9000000000000" :step="1"/></div>
     </div>
     <div class="mt-2 text-xs text-gray-500">最终售价 = 上游 SKU 原价 × 汇率转 USDT ×（1 + 百分比）+ 固定加价。固定加价以 0.01 USDT 为单位；价格按分四舍五入。</div>
    </a-form-item>
    <a-form-item label="上游币种兑 USDT 汇率" extra="填写 1 单位上游币种可兑换多少 USDT。例如上游为 CNY 时可填 7.20；上游币种为 USDT 时自动按 1 计算。非 USDT 币种未配置汇率将拒绝同步，避免错误定价。">
     <a-input-number v-model:value="settings.exchangeRateToUSDT" class="w-full" :min="0" :max="1000000" :step="0.01" placeholder="例如 CNY → USDT：7.20"/>
    </a-form-item>
    <a-form-item label="自动同步间隔">
     <a-select v-model:value="settings.syncIntervalMinutes" class="w-full">
      <a-select-option :value="5">每 5 分钟</a-select-option><a-select-option :value="15">每 15 分钟</a-select-option><a-select-option :value="30">每 30 分钟</a-select-option><a-select-option :value="60">每小时</a-select-option><a-select-option :value="360">每 6 小时</a-select-option><a-select-option :value="1440">每天</a-select-option>
     </a-select>
    </a-form-item>
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
     <a-form-item label="同步批次大小（条）" extra="每页处理 20–50 个上游商品；默认 25。"><a-input-number v-model:value="settings.syncBatchSize" class="w-full" :min="20" :max="50" :step="5"/></a-form-item>
     <a-form-item label="批次间隔（毫秒）" extra="每批提交后暂停 250–10000 毫秒；默认 1000。"><a-input-number v-model:value="settings.syncBatchDelayMs" class="w-full" :min="250" :max="10000" :step="250"/></a-form-item>
    </div>
    <a-form-item label="启用上游售卖">
     <div class="flex flex-wrap items-center gap-3">
      <a-switch v-model:checked="settings.enabled"  :disabled="settings.enabled||settings.catalogStale||settings.syncStatus!=='success'"/>
      <span class="text-sm text-gray-500">先保存凭证并成功同步一次后才能启用；上游售卖启用期间不能直接修改站点、凭证或价格策略；请先停用并清除上游目录，再修改配置、重新同步后启用。启用后 Bot 只展示上游目录；停用操作会先切回本地商品，再清除上游目录和图片，历史订单与资金流水保留。</span>
     </div>
    </a-form-item>
    <a-divider />
    <a-form-item label="上游采购回调地址" extra="请填写本站公网 HTTPS 地址，路径固定为 /api/v1/upstream/callback。配置后上游交付完成会主动回调；轮询仍作为兜底。">
     <a-input v-model:value="settings.callbackUrl" placeholder="https://你的域名/api/v1/upstream/callback" autocomplete="url"/>
    </a-form-item>
    <a-form-item label="采购查单轮询间隔（秒）"><a-input-number v-model:value="settings.procurementPollIntervalSeconds" class="w-full" :min="5" :max="3600" :step="5"/></a-form-item>
    <a-form-item label="采购等待超时（分钟）"><a-input-number v-model:value="settings.procurementTimeoutMinutes" class="w-full" :min="1" :max="1440" :step="1"/></a-form-item>
    <a-form-item label="供应商余额低于此值时告警" extra="按上游账户自身币种填写，0 表示关闭低余额阈值告警。"><a-input v-model:value="settings.supplierLowBalanceThreshold" placeholder="例如 100.00"/></a-form-item>
    <a-alert class="mb-4" type="info" show-icon message="采购将在本地订单支付确认后异步执行；上游回调与查单轮询共同推进交付。采购失败只进入人工处理队列，不自动退款或修改买家已支付资金。" />
    <div class="flex flex-wrap items-center gap-3">
     <a-button type="primary" :loading="saving" @click="save">保存配置</a-button>
     <a-button :loading="testing" :disabled="saving||testing||syncing||clearCredentials||(!settings.merchantIdConfigured&&!merchantId)||(!settings.merchantSecretConfigured&&!merchantSecret)" @click="testConnection">测试连接</a-button>
     <a-button :loading="syncing" :disabled="saving||testing||syncing||clearCredentials||(!settings.merchantIdConfigured&&!merchantId)||(!settings.merchantSecretConfigured&&!merchantSecret)" @click="syncNow">立即同步商品</a-button>
     <a-button v-if="syncing&&syncProgress.status!=='cleaning'" danger :disabled="!syncJobId" @click="cancelSync">取消同步</a-button>
     <a-popconfirm title="停用后将删除全部上游分类、商品、SKU 和不再被本地商品引用的上游图片；历史订单及资金流水保留。此操作不可恢复，是否继续？" ok-text="确认清除" cancel-text="取消" @confirm="disableUpstream"><a-button danger :disabled="!settings.enabled||syncing">停用并清除上游数据</a-button></a-popconfirm>
     <a-button :disabled="saving||testing||syncing" @click="load">重新加载</a-button>
    </div>
   </a-form>
  </a-card>
  <a-card :bordered="false" title="上游采购人工处理队列">
   <template #extra><a-button size="small" :loading="procurementLoading" @click="loadProcurements">刷新队列</a-button></template>
   <a-table :data-source="procurements" :loading="procurementLoading" :pagination="{pageSize:8}" row-key="localOrderId" :scroll="{x:760}">
    <a-table-column title="本地订单号" data-index="localOrderNo" :width="190"/>
    <a-table-column title="上游订单号" data-index="externalOrderNo" :width="170"/>
    <a-table-column title="状态" :width="130"><template #default="{record}"><a-tag :color="record.status==='failed'||record.status==='uncertain'?'error':record.status==='completed'?'success':'processing'">{{procurementStatus(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="重试次数" data-index="requestAttempts" :width="90"/>
    <a-table-column title="异常原因" data-index="lastError" :ellipsis="true" :width="260"/>
    <a-table-column title="操作" :fixed="'right'" :width="170"><template #default="{record}">
     <div class="flex gap-2" v-if="record.status==='failed'||record.status==='uncertain'">
      <a-button size="small" type="primary" :loading="actionLoading===record.localOrderId" @click="retryProcurement(record)">{{record.externalOrderNo ? "查单恢复" : "手动重试采购"}}</a-button>
      <a-button size="small" danger :disabled="!!actionLoading" @click="openRefund(record)">退款</a-button>
     </div><span v-else class="text-xs text-gray-400">—</span>
    </template></a-table-column>
   </a-table>
   <a-alert class="mt-3" type="info" show-icon message="采购异常不会自动退款或改变原始支付记录。请先核实上游订单受理状态；已存在上游订单号的失败记录点击“查单恢复”只查询该上游订单，不会再次创建采购单；没有上游订单号的记录使用原本地订单号作为幂等键手动重试。" />
  </a-card>
  <a-modal v-model:open="refundVisible" title="确认上游采购失败退款" :confirm-loading="!!actionLoading" @ok="submitRefund">
   <a-alert type="warning" show-icon message="退款将按本地订单金额记入买家站内余额，并生成独立退款账本和管理员审计记录。" />
   <a-form-item class="mt-4" label="退款原因"><a-textarea v-model:value="refundReason" :rows="3" :maxlength="500" show-count placeholder="填写已确认无法履约的原因"/></a-form-item>
  </a-modal>
  <a-card :bordered="false" title="连接与同步状态">
   <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
    <div class="rounded-lg border border-gray-200 p-4">
     <div class="mb-2 text-sm text-gray-500">连接测试</div>
     <a-tag :color="settings.lastTestStatus==='success'?'success':settings.lastTestStatus==='failed'?'error':'default'">{{testLabel}}</a-tag>
     <div v-if="settings.lastTestAt" class="mt-2 text-xs text-gray-500">测试时间：{{settings.lastTestAt}}</div>
     <a-alert v-if="settings.lastError" class="mt-3" type="error" show-icon :message="settings.lastError"/>
     <div v-if="testResult" class="mt-3 text-sm">站点：{{testResult.siteName}} · 协议：{{testResult.protocolVersion}} · 余额：{{testResult.balance}} {{testResult.currency}}</div>
    </div>
    <div class="rounded-lg border border-gray-200 p-4">
     <div class="mb-2 text-sm text-gray-500">商品同步</div>
     <a-tag color="default">{{syncLabel}}</a-tag>
     <div v-if="settings.syncAt" class="mt-2 text-xs text-gray-500">最近成功同步：{{settings.syncAt}}</div>
     <div v-if="settings.syncAttemptAt" class="mt-1 text-xs text-gray-500">最近尝试：{{settings.syncAttemptAt}}</div>
     <div v-if="syncing" class="mt-2 text-xs text-gray-600">任务状态：{{syncProgress.status}} · 已处理 {{syncProgress.processedItems}} / {{syncProgress.totalItems}} · 当前页 {{syncProgress.currentPage}} · 错误 {{syncProgress.errorCount}}</div>
     <a-alert v-if="syncProgress.lastError" class="mt-2" type="warning" show-icon :message="syncProgress.lastError"/>
     <a-alert v-if="settings.syncError" class="mt-3" type="warning" show-icon :message="settings.syncError"/>
     <div v-else class="mt-2 text-xs text-gray-500">自动同步按上方间隔运行；同步失败会保留上一次成功的数据。</div>
     <div v-if="syncResult" class="mt-3 text-sm">本次同步：{{syncResult.categories}} 个分类 · {{syncResult.products}} 个 SKU 商品 · 图片失败 {{syncResult.imagesFailed}} 张</div>
     <a-alert v-if="syncResult?.warning" class="mt-3" type="warning" show-icon :message="syncResult.warning"/>
    </div>
   </div>
  </a-card>
 </div>
</template>
