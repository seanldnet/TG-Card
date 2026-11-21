<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { withAdminActionConfirmation } from '#/utils/admin-action-confirm';

interface BonusTier { minAmountAtomic: number; bonusType: 'fixed' | 'percent'; bonusValue: number; }
interface BonusRule { enabled: boolean; tiers: BonusTier[]; }

interface PaymentSettings {
  apiKeyConfigured: boolean;
  receiveAddress: string;
  usdtContract: string;
  enabled: boolean;
  epusdtBaseUrl: string;
  epusdtPid: string;
  epusdtSecretConfigured: boolean;
  epusdtEnabled: boolean;
  epusdtNotifyUrl: string;
  gmpayBaseUrl: string;
  gmpayPid: string;
  gmpaySecretConfigured: boolean;
  gmpayEnabled: boolean;
  gmpayNotifyUrl: string;
}

const loading = ref(false);
const bonusSaving = ref(false);
const bonusRule = reactive<BonusRule>({enabled:false,tiers:[]});
const saving = ref(false);
const apiKey = ref('');
const clearApiKey = ref(false);
const epusdtSecret = ref('');
const clearEpusdtSecret = ref(false);
const gmpaySecret = ref('');
const clearGmpaySecret = ref(false);
const settings = reactive<PaymentSettings>({
  apiKeyConfigured: false,
  receiveAddress: '',
  usdtContract: 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
  enabled: false,
  epusdtBaseUrl: '', epusdtPid: '', epusdtSecretConfigured: false, epusdtEnabled: false, epusdtNotifyUrl: '',
  gmpayBaseUrl: '', gmpayPid: '', gmpaySecretConfigured: false, gmpayEnabled: false, gmpayNotifyUrl: '',
});

async function loadBonusRule(){
  try { const data=await requestClient.get<BonusRule>('/settings/recharge-bonus'); Object.assign(bonusRule,data); bonusRule.tiers=data.tiers||[]; }
  catch { message.error('充值赠送规则加载失败'); }
}
async function saveBonusRule(){
  bonusSaving.value=true;
  try { const payload={enabled:bonusRule.enabled,tiers:bonusRule.tiers}; const data=await withAdminActionConfirmation({actionKey:'settings.recharge-bonus.update',method:'PUT',path:'/api/settings/recharge-bonus',payload,run:(token)=>requestClient.put<BonusRule>('/settings/recharge-bonus',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)}); Object.assign(bonusRule,data); bonusRule.tiers=data.tiers||[]; message.success('充值赠送规则已保存'); }
  catch(error:any){ message.error(error?.response?.data?.message||'保存充值赠送规则失败'); }
  finally { bonusSaving.value=false; }
}
function addBonusTier(){bonusRule.tiers.push({minAmountAtomic:10000000,bonusType:'fixed',bonusValue:1000000});}
function removeBonusTier(index:number){bonusRule.tiers.splice(index,1);}

async function loadSettings() {
  loading.value = true;
  try {
    const data = await requestClient.get<PaymentSettings>('/settings/payment');
    Object.assign(settings, data);
  } catch {
    message.error('支付设置加载失败');
  } finally {
    loading.value = false;
  }
}

function settingsPayload() {
  return {
    apiKey: apiKey.value, clearApiKey: clearApiKey.value,
    receiveAddress: settings.receiveAddress, usdtContract: settings.usdtContract, enabled: settings.enabled,
    epusdtBaseUrl: settings.epusdtBaseUrl, epusdtPid: settings.epusdtPid,
    epusdtSecret: epusdtSecret.value, clearEpusdtSecret: clearEpusdtSecret.value, epusdtEnabled: settings.epusdtEnabled,
    gmpayBaseUrl: settings.gmpayBaseUrl, gmpayPid: settings.gmpayPid,
    gmpaySecret: gmpaySecret.value, clearGmpaySecret: clearGmpaySecret.value, gmpayEnabled: settings.gmpayEnabled,
  };
}

async function saveGateway(provider: 'epusdt' | 'gmpay') {
  saving.value = true;
  try {
    const payload = settingsPayload();
    const data = await withAdminActionConfirmation({
      actionKey: 'settings.payment.update', method: 'PUT', path: '/api/settings/payment', payload,
      run: (token) => requestClient.put<PaymentSettings>('/settings/payment', payload, token ? { headers: { 'X-Admin-Action-Confirmation': token } } : undefined),
    });
    Object.assign(settings, data);
    apiKey.value = ''; clearApiKey.value = false;
    epusdtSecret.value = ''; clearEpusdtSecret.value = false;
    gmpaySecret.value = ''; clearGmpaySecret.value = false;
    message.success(provider === 'epusdt' ? 'Epusdt 配置已保存并应用' : 'GMPay Edge 配置已保存并应用');
  } catch (error: any) { message.error(error?.response?.data?.message || '支付网关配置保存失败'); }
  finally { saving.value = false; }
}

async function saveSettings() {
  saving.value = true;
  try {
    const payload = settingsPayload();
    const data = await withAdminActionConfirmation({
      actionKey: 'settings.payment.update',
      method: 'PUT',
      path: '/api/settings/payment',
      payload,
      run: (token) => requestClient.put<PaymentSettings>('/settings/payment', payload, token ? { headers: { 'X-Admin-Action-Confirmation': token } } : undefined),
    });
    Object.assign(settings, data);
    apiKey.value = '';
    clearApiKey.value = false;
    epusdtSecret.value = '';
    clearEpusdtSecret.value = false;
    gmpaySecret.value = '';
    clearGmpaySecret.value = false;
    message.success('支付设置已保存');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败，请检查配置');
  } finally {
    saving.value = false;
  }
}

onMounted(()=>{void loadSettings();void loadBonusRule()});
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
    <a-card :bordered="false" title="支付方式">
      <div class="grid grid-cols-1 gap-4 md:grid-cols-1">
        <div class="flex items-center justify-between rounded-lg border border-gray-200 p-4">
          <div><div class="font-medium">内置 USDT（TRC20）</div><div class="text-xs text-gray-500">链上收款</div></div>
          <a-switch v-model:checked="settings.enabled" />
        </div>
      </div>
      <div class="mt-4 flex justify-end"><a-button type="primary" :loading="saving" @click="saveSettings">保存并应用</a-button></div>
    </a-card>
    <a-card :bordered="false" :loading="loading" title="USDT - TRC20 收款配置">
      <template #extra>
        <a-tag :color="settings.enabled ? 'success' : 'default'">
          {{ settings.enabled ? '收款已启用' : '收款未启用' }}
        </a-tag>
      </template>
      
      <a-form layout="vertical">
        <a-form-item label="TronGrid API Key">
          <a-input-password
            v-model:value="apiKey"
            autocomplete="new-password"
            placeholder="输入 TronGrid API Key"
          />
          <div class="mt-2 flex items-center gap-2">
            <a-tag :color="settings.apiKeyConfigured ? 'success' : 'warning'">
              {{ settings.apiKeyConfigured ? 'API Key 已配置' : '尚未配置 API Key' }}
            </a-tag>
            <a-checkbox v-model:checked="clearApiKey">清除已保存的 API Key</a-checkbox>
          </div>
        </a-form-item>
        <a-form-item label="TRON 收款地址" required>
          <a-input
            v-model:value="settings.receiveAddress"
            autocomplete="off"
            placeholder="T 开头的 TRON 主网地址"
          />
        </a-form-item>
        <a-form-item label="USDT TRC20 合约地址">
          <a-input v-model:value="settings.usdtContract" autocomplete="off" />
        </a-form-item>
        <a-divider />

        <a-alert
          class="mb-4"
          type="warning"
          show-icon
          message="安全提醒"
          description="请只填写商户自己的收款地址。系统仅监听到账，不需要钱包私钥；少付、多付、过期转账仍需人工对账。"
        />
      </a-form>
    </a-card>
    <a-card :bordered="false" :loading="loading" title="Epusdt 网关">
      <template #extra><div class="flex items-center gap-2"><a-tag :color="settings.epusdtEnabled ? 'success' : 'default'">{{ settings.epusdtEnabled ? '已启用' : '未启用' }}</a-tag><a-switch v-model:checked="settings.epusdtEnabled" /></div></template>
      <a-form layout="vertical">
        <a-form-item label="Epusdt 服务地址" required><a-input v-model:value="settings.epusdtBaseUrl" placeholder="https://pay.example.com" autocomplete="url"/></a-form-item>
        <a-form-item label="Epusdt 商户 PID" required><a-input v-model:value="settings.epusdtPid" autocomplete="off"/></a-form-item>
        <a-form-item label="Epusdt Secret"><a-input-password v-model:value="epusdtSecret" autocomplete="new-password" placeholder="输入 Epusdt 商户 Secret"/><div class="mt-2 flex items-center gap-2"><a-tag :color="settings.epusdtSecretConfigured ? 'success' : 'warning'">{{ settings.epusdtSecretConfigured ? 'Secret 已配置' : 'Secret 尚未配置' }}</a-tag><a-checkbox v-model:checked="clearEpusdtSecret">清除已保存 Secret</a-checkbox></div></a-form-item>
        <div class="mt-4 flex justify-end"><a-button type="primary" :loading="saving" @click="saveGateway('epusdt')">保存 Epusdt 配置</a-button></div>
      </a-form>
    </a-card>
    <a-card :bordered="false" :loading="loading" title="GMPay Edge 网关">
      <template #extra><div class="flex items-center gap-2"><a-tag :color="settings.gmpayEnabled ? 'success' : 'default'">{{ settings.gmpayEnabled ? '已启用' : '未启用' }}</a-tag><a-switch v-model:checked="settings.gmpayEnabled" /></div></template>
      <a-form layout="vertical">
        <a-form-item label="GMPay Edge 服务地址" required><a-input v-model:value="settings.gmpayBaseUrl" placeholder="https://gmpay.example.com" autocomplete="url"/></a-form-item>
        <a-form-item label="GMPay Edge 商户 PID" required><a-input v-model:value="settings.gmpayPid" autocomplete="off"/></a-form-item>
        <a-form-item label="GMPay Edge Secret"><a-input-password v-model:value="gmpaySecret" autocomplete="new-password" placeholder="输入 GMPay Edge 商户 Secret"/><div class="mt-2 flex items-center gap-2"><a-tag :color="settings.gmpaySecretConfigured ? 'success' : 'warning'">{{ settings.gmpaySecretConfigured ? 'Secret 已配置' : 'Secret 尚未配置' }}</a-tag><a-checkbox v-model:checked="clearGmpaySecret">清除已保存 Secret</a-checkbox></div></a-form-item>
        <div class="mt-4 flex justify-end"><a-button type="primary" :loading="saving" @click="saveGateway('gmpay')">保存 GMPay 配置</a-button></div>
      </a-form>
    </a-card>
    <a-card :bordered="false" title="充值赠送规则">
      
      <div class="mb-4 flex flex-wrap items-center gap-3"><a-switch v-model:checked="bonusRule.enabled"/><span>启用充值赠送</span><a-button size="small" @click="addBonusTier">添加阶梯</a-button></div>
      <div v-for="(tier,index) in bonusRule.tiers" :key="index" class="mb-3 grid grid-cols-1 gap-2 rounded-lg border border-gray-200 p-3 md:grid-cols-3">
       <a-form-item label="充值满（USDT）" class="mb-0"><a-input-number v-model:value="tier.minAmountAtomic" :min="1000000" :step="1000000" class="w-full" :formatter="(v:any)=>String(Number(v||0)/1000000)" :parser="(v:any)=>Number(v)*1000000"/></a-form-item>
       <a-form-item label="赠送方式" class="mb-0"><a-select v-model:value="tier.bonusType"><a-select-option value="fixed">固定赠送</a-select-option><a-select-option value="percent">按比例赠送</a-select-option></a-select></a-form-item>
       <a-form-item :label="tier.bonusType==='fixed'?'赠送金额（USDT）':'赠送比例（%）'" class="mb-0"><a-input-number v-model:value="tier.bonusValue" :min="1" :max="tier.bonusType==='percent'?100:100000000" class="w-full" :formatter="(v:any)=>tier.bonusType==='fixed'?String(Number(v||0)/1000000):String(Number(v||0)/100)" :parser="(v:any)=>Math.round(Number(v)*(tier.bonusType==='fixed'?1000000:100))"/></a-form-item>
       <div class="md:col-span-3 flex justify-end"><a-button danger size="small" @click="removeBonusTier(index)">删除此阶梯</a-button></div>
      </div>
      <a-empty v-if="bonusRule.tiers.length===0" description="尚未配置阶梯规则，开启后添加至少一档规则"/>
      <div class="mt-4 flex justify-end"><a-button type="primary" :loading="bonusSaving" @click="saveBonusRule">保存充值赠送规则</a-button></div>
    </a-card>
  </div>
</template>
