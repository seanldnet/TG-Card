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
  lastTestStatus: string;
  lastTestAt: string;
  lastError: string;
  syncStatus: string;
  syncAt: string;
  syncError: string;
  updatedAt: string;
}
const loading=ref(false),saving=ref(false),testing=ref(false);
const merchantId=ref(''),merchantSecret=ref(''),clearCredentials=ref(false);
const settings=reactive<UpstreamSettings>({
 provider:'dujiao_next',baseUrl:'',merchantIdMasked:'',merchantIdConfigured:false,
 merchantSecretConfigured:false,enabled:false,lastTestStatus:'not_tested',lastTestAt:'',
 lastError:'',syncStatus:'not_started',syncAt:'',syncError:'',updatedAt:'',
});
const testResult=ref<{siteName:string;protocolVersion:string;balance:string;currency:string}|null>(null);
const testLabel=computed(()=>({success:'连接成功',failed:'连接失败',not_tested:'尚未测试'}[settings.lastTestStatus]||settings.lastTestStatus));
const syncLabel=computed(()=>({not_started:'尚未同步',idle:'空闲',running:'同步中',success:'同步成功',failed:'同步失败'}[settings.syncStatus]||settings.syncStatus));
async function load(){
 loading.value=true;
 try{const data=await requestClient.get<UpstreamSettings>('/upstream/config');Object.assign(settings,data);}
 catch(error:any){message.error(error?.response?.data?.message||'上游配置加载失败');}
 finally{loading.value=false;}
}
async function save(){
 saving.value=true;
 try{
  const data=await requestClient.put<UpstreamSettings>('/upstream/config',{
   provider:settings.provider,baseUrl:settings.baseUrl,merchantId:merchantId.value,
   merchantSecret:merchantSecret.value,clearCredentials:clearCredentials.value,enabled:settings.enabled,
  });
  Object.assign(settings,data);merchantId.value='';merchantSecret.value='';clearCredentials.value=false;testResult.value=null;
  message.success('上游配置已安全保存');
 }catch(error:any){message.error(error?.response?.data?.message||'上游配置保存失败');}
 finally{saving.value=false;}
}
async function testConnection(){
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
 <div class="mx-auto w-full min-w-0 max-w-none p-3 md:p-6">
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
    <a-form-item label="启用上游商品来源">
     <div class="flex flex-wrap items-center gap-3">
      <a-switch v-model:checked="settings.enabled"/>
      <span class="text-sm text-gray-500">PR B 完成商品同步与来源切换后，启用状态才会改变实际商品展示。</span>
     </div>
    </a-form-item>
    <a-alert class="mb-4" type="warning" show-icon message="启用上游开关目前只保存配置，不会在 PR A 阶段改变商品展示或触发采购。目录同步和采购转发将在后续独立 PR 实现。" />
    <div class="flex flex-wrap items-center gap-3">
     <a-button type="primary" :loading="saving" @click="save">保存配置</a-button>
     <a-button :loading="testing" :disabled="saving||testing||!settings.merchantIdConfigured&&!merchantId||!settings.merchantSecretConfigured&&!merchantSecret" @click="testConnection">测试连接</a-button>
     <a-button :disabled="saving||testing" @click="load">重新加载</a-button>
    </div>
   </a-form>
  </a-card>
  <a-card class="mt-4" :bordered="false" title="连接与同步状态">
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
     <div v-if="settings.syncAt" class="mt-2 text-xs text-gray-500">最近同步：{{settings.syncAt}}</div>
     <a-alert v-if="settings.syncError" class="mt-3" type="warning" show-icon :message="settings.syncError"/>
     <div v-else class="mt-2 text-xs text-gray-500">同步任务将在 PR B 实现；当前尚未同步任何上游商品。</div>
    </div>
   </div>
  </a-card>
 </div>
</template>
