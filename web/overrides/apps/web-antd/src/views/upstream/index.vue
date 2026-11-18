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
  exchangeRateToUSDT: number;
  callbackUrl: string;
  purchasePollIntervalSeconds: number;
  purchaseTimeoutMinutes: number;
  supplierBalanceAlertThreshold: string;
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
 merchantSecretConfigured:false,enabled:false,catalogStale:true,markupPercent:0,markupFixedMinor:0,syncIntervalMinutes:15,exchangeRateToUSDT:0,callbackUrl:'',purchasePollIntervalSeconds:15,purchaseTimeoutMinutes:30,supplierBalanceAlertThreshold:'10',lastTestStatus:'not_tested',lastTestAt:'',
 lastError:'',syncStatus:'not_started',syncAt:'',syncAttemptAt:'',syncError:'',updatedAt:'',
});
const testResult=ref<{siteName:string;protocolVersion:string;balance:string;currency:string}|null>(null);
const syncResult=ref<{categories:number;products:number;imagesFailed:number;warning:string}|null>(null);
const testLabel=computed(()=>{const labels:Record<string,string>={success:'连接成功',failed:'连接失败',not_tested:'尚未测试'};return labels[settings.lastTestStatus]||settings.lastTestStatus;});
const syncLabel=computed(()=>{const labels:Record<string,string>={not_started:'尚未同步',idle:'空闲',running:'同步中',success:'同步成功',failed:'同步失败'};return labels[settings.syncStatus]||settings.syncStatus;});
async function load(){
 loading.value=true;
 try{const data=await requestClient.get<UpstreamSettings>('/upstream/config');Object.assign(settings,data);}
 catch(error:any){message.error(error?.response?.data?.message||'上游配置加载失败');}
 finally{loading.value=false;}
}
async function save():Promise<boolean>{
 saving.value=true;
 try{
  const requestedEnabled=settings.enabled;
  const data=await requestClient.put<UpstreamSettings>('/upstream/config',{
   provider:settings.provider,baseUrl:settings.baseUrl,merchantId:merchantId.value,
   merchantSecret:merchantSecret.value,clearCredentials:clearCredentials.value,enabled:settings.enabled,
   markupPercent:settings.markupPercent,markupFixedMinor:settings.markupFixedMinor,syncIntervalMinutes:settings.syncIntervalMinutes,exchangeRateToUSDT:settings.exchangeRateToUSDT,callbackUrl:settings.callbackUrl,purchasePollIntervalSeconds:settings.purchasePollIntervalSeconds,purchaseTimeoutMinutes:settings.purchaseTimeoutMinutes,supplierBalanceAlertThreshold:settings.supplierBalanceAlertThreshold,
  });
  Object.assign(settings,data);merchantId.value='';merchantSecret.value='';clearCredentials.value=false;testResult.value=null;
  if(requestedEnabled&&!settings.enabled){message.info('配置已保存；站点、凭证或价格策略有变化，上游展示已保持关闭，请重新同步后再启用。');}else{message.success('上游配置已安全保存');}return true;
 }catch(error:any){message.error(error?.response?.data?.message||'上游配置保存失败');return false;}
 finally{saving.value=false;}
}
async function syncNow(){
 if(clearCredentials.value){message.warning('请先保存配置后再同步');return;}
 if(merchantId.value||merchantSecret.value||!settings.merchantIdConfigured||!settings.merchantSecretConfigured){const saved=await save();if(!saved)return;}
 syncing.value=true;syncResult.value=null;
 try{
  const data=await requestClient.post<any>('/upstream/sync',{});
  syncResult.value={categories:data.categories||0,products:data.products||0,imagesFailed:data.imagesFailed||0,warning:data.warning||''};
  await load();message.success('上游商品同步完成');
 }catch(error:any){await load();message.error(error?.response?.data?.message||'上游商品同步失败，旧目录数据已保留');}
 finally{syncing.value=false;}
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
onMounted(()=>{void load();});
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <a-card :bordered="false" :loading="loading" title="上游对接">
   <template #extra><a-tag :color="settings.enabled?'success':'default'">{{settings.enabled?'上游已启用':'上游未启用'}}</a-tag></template>
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
    <a-divider/>
    <a-form-item label="采购回调地址" extra="填写 TG-Card 对外可访问的 HTTPS 地址，路径必须为 /api/v1/upstream/callback。留空时系统仍会使用主动查单轮询。">
     <a-input v-model:value="settings.callbackUrl" placeholder="https://your-card-domain.example/api/v1/upstream/callback" autocomplete="url"/>
    </a-form-item>
    <a-form-item label="采购查单轮询间隔与超时">
     <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <div><div class="mb-1 text-xs text-gray-500">轮询间隔（秒）</div><a-input-number v-model:value="settings.purchasePollIntervalSeconds" class="w-full" :min="5" :max="300" :step="5"/></div>
      <div><div class="mb-1 text-xs text-gray-500">采购总超时（分钟）</div><a-input-number v-model:value="settings.purchaseTimeoutMinutes" class="w-full" :min="1" :max="1440" :step="1"/></div>
     </div>
    </a-form-item>
    <a-form-item label="供应商余额低额预警阈值" extra="单位为供应商 ping 返回的币种；余额低于此值时每日对账日报会标记并通过风险通知提醒 Owner。默认 10。">
     <a-input-number v-model:value="settings.supplierBalanceAlertThreshold" class="w-full" :min="0.000001" :max="1000000000" :step="1"/>
    </a-form-item>
    <a-form-item label="启用上游商品来源">
     <div class="flex flex-wrap items-center gap-3">
      <a-switch v-model:checked="settings.enabled" :disabled="settings.catalogStale||settings.syncStatus!=='success'&&!settings.enabled"/>
      <span class="text-sm text-gray-500">先保存凭证并成功同步一次后才能启用；配置、凭证或价格策略变化会自动标记目录过期并关闭上游展示，重新同步成功后才能再次启用。启用后 Bot 切换到上游商品目录，本地旧商品在后台保留并标记为隐藏。</span>
     </div>
    </a-form-item>
    <a-alert class="mb-4" type="info" show-icon message="上游采购在支付确认后异步执行；建议配置公网 HTTPS 回调地址，系统同时保留主动查单轮询。采购失败不会自动退款或扣减买家余额，将进入人工处理。" />
    <div class="flex flex-wrap items-center gap-3">
     <a-button type="primary" :loading="saving" @click="save">保存配置</a-button>
     <a-button :loading="testing" :disabled="saving||testing||syncing||clearCredentials||(!settings.merchantIdConfigured&&!merchantId)||(!settings.merchantSecretConfigured&&!merchantSecret)" @click="testConnection">测试连接</a-button>
     <a-button :loading="syncing" :disabled="saving||testing||syncing||clearCredentials||(!settings.merchantIdConfigured&&!merchantId)||(!settings.merchantSecretConfigured&&!merchantSecret)" @click="syncNow">立即同步商品</a-button>
     <a-button :disabled="saving||testing||syncing" @click="load">重新加载</a-button>
    </div>
   </a-form>
  </a-card>
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
     <a-alert v-if="settings.syncError" class="mt-3" type="warning" show-icon :message="settings.syncError"/>
     <div v-else class="mt-2 text-xs text-gray-500">自动同步按上方间隔运行；同步失败会保留上一次成功的数据。</div>
     <div v-if="syncResult" class="mt-3 text-sm">本次同步：{{syncResult.categories}} 个分类 · {{syncResult.products}} 个 SKU 商品 · 图片失败 {{syncResult.imagesFailed}} 张</div>
     <a-alert v-if="syncResult?.warning" class="mt-3" type="warning" show-icon :message="syncResult.warning"/>
    </div>
   </div>
  </a-card>
 </div>
</template>
