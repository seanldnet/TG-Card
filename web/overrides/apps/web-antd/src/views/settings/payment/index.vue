<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
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
  catch { message.error($t('tg-card.paymentSettings.bonusLoadFailed')); }
}
async function saveBonusRule(){
  bonusSaving.value=true;
  try { const payload={enabled:bonusRule.enabled,tiers:bonusRule.tiers}; const data=await withAdminActionConfirmation({actionKey:'settings.recharge-bonus.update',method:'PUT',path:'/api/settings/recharge-bonus',payload,run:(token)=>requestClient.put<BonusRule>('/settings/recharge-bonus',payload,token?{headers:{'X-Admin-Action-Confirmation':token}}:undefined)}); Object.assign(bonusRule,data); bonusRule.tiers=data.tiers||[]; message.success($t('tg-card.paymentSettings.bonusSaved')); }
  catch(error:any){ message.error(error?.response?.data?.message||$t('tg-card.paymentSettings.bonusSaveFailed')); }
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
    message.error($t('tg-card.paymentSettings.loadFailed'));
  } finally {
    loading.value = false;
  }
}

async function settingsPayload(scope: 'tron' | 'epusdt' | 'gmpay') {
  const saved = await requestClient.get<PaymentSettings>('/settings/payment');
  return {
    apiKey: scope === 'tron' ? apiKey.value : '',
    clearApiKey: scope === 'tron' ? clearApiKey.value : false,
    receiveAddress: scope === 'tron' ? settings.receiveAddress : saved.receiveAddress,
    usdtContract: scope === 'tron' ? settings.usdtContract : saved.usdtContract,
    enabled: scope === 'tron' ? settings.enabled : saved.enabled,
    epusdtBaseUrl: scope === 'epusdt' ? settings.epusdtBaseUrl : saved.epusdtBaseUrl,
    epusdtPid: scope === 'epusdt' ? settings.epusdtPid : saved.epusdtPid,
    epusdtSecret: scope === 'epusdt' ? epusdtSecret.value : '',
    clearEpusdtSecret: scope === 'epusdt' ? clearEpusdtSecret.value : false,
    epusdtEnabled: scope === 'epusdt' ? settings.epusdtEnabled : saved.epusdtEnabled,
    gmpayBaseUrl: scope === 'gmpay' ? settings.gmpayBaseUrl : saved.gmpayBaseUrl,
    gmpayPid: scope === 'gmpay' ? settings.gmpayPid : saved.gmpayPid,
    gmpaySecret: scope === 'gmpay' ? gmpaySecret.value : '',
    clearGmpaySecret: scope === 'gmpay' ? clearGmpaySecret.value : false,
    gmpayEnabled: scope === 'gmpay' ? settings.gmpayEnabled : saved.gmpayEnabled,
  };
}

async function saveGateway(provider: 'epusdt' | 'gmpay') {
  saving.value = true;
  try {
    const payload = await settingsPayload(provider);
    const data = await withAdminActionConfirmation({
      actionKey: 'settings.payment.update', method: 'PUT', path: '/api/settings/payment', payload,
      run: (token) => requestClient.put<PaymentSettings>('/settings/payment', payload, token ? { headers: { 'X-Admin-Action-Confirmation': token } } : undefined),
    });
    if (provider === 'epusdt') {
      settings.epusdtSecretConfigured = data.epusdtSecretConfigured;
      epusdtSecret.value = ''; clearEpusdtSecret.value = false;
    } else {
      settings.gmpaySecretConfigured = data.gmpaySecretConfigured;
      gmpaySecret.value = ''; clearGmpaySecret.value = false;
    }
    message.success(provider === 'epusdt' ? $t('tg-card.paymentSettings.epusdtSaved') : $t('tg-card.paymentSettings.gmpaySaved'));
  } catch (error: any) { message.error(error?.response?.data?.message || $t('tg-card.paymentSettings.gatewaySaveFailed')); }
  finally { saving.value = false; }
}

async function saveSettings() {
  saving.value = true;
  try {
    const payload = await settingsPayload('tron');
    const data = await withAdminActionConfirmation({
      actionKey: 'settings.payment.update',
      method: 'PUT',
      path: '/api/settings/payment',
      payload,
      run: (token) => requestClient.put<PaymentSettings>('/settings/payment', payload, token ? { headers: { 'X-Admin-Action-Confirmation': token } } : undefined),
    });
    settings.apiKeyConfigured = data.apiKeyConfigured;
    settings.receiveAddress = data.receiveAddress;
    settings.usdtContract = data.usdtContract;
    settings.enabled = data.enabled;
    apiKey.value = '';
    clearApiKey.value = false;
    message.success($t('tg-card.paymentSettings.tronSaved'));
  } catch (error: any) {
    message.error(error?.response?.data?.message || $t('tg-card.paymentSettings.saveFailed'));
  } finally {
    saving.value = false;
  }
}

onMounted(()=>{void loadSettings();void loadBonusRule()});
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
    <a-card :bordered="false" :loading="loading" :title="$t('tg-card.paymentSettings.tronTitle')">
      <template #extra><div class="flex items-center gap-2"><a-tag :color="settings.enabled ? 'success' : 'default'">{{ settings.enabled ? $t('tg-card.paymentSettings.enabled') : $t('tg-card.paymentSettings.disabled') }}</a-tag><a-switch v-model:checked="settings.enabled" /></div></template>
      
      <a-form layout="vertical">
        <a-form-item label="TronGrid API Key">
          <a-input-password
            v-model:value="apiKey"
            autocomplete="new-password"
            :placeholder="$t('tg-card.paymentSettings.tronGridKey')"
          />
          <div class="mt-2 flex items-center gap-2">
            <a-tag :color="settings.apiKeyConfigured ? 'success' : 'warning'">
              {{ settings.apiKeyConfigured ? $t('tg-card.paymentSettings.apiKeyConfigured') : $t('tg-card.paymentSettings.apiKeyMissing') }}
            </a-tag>
            <a-checkbox v-model:checked="clearApiKey">{{$t('tg-card.paymentSettings.clearApiKey')}}</a-checkbox>
          </div>
        </a-form-item>
        <a-form-item :label="$t('tg-card.paymentSettings.tronAddress')" required>
          <a-input
            v-model:value="settings.receiveAddress"
            autocomplete="off"
            :placeholder="$t('tg-card.paymentSettings.tronAddressPlaceholder')"
          />
        </a-form-item>
        <a-form-item :label="$t('tg-card.paymentSettings.contractAddress')">
          <a-input v-model:value="settings.usdtContract" autocomplete="off" />
        </a-form-item>
        <a-divider />

        <a-alert
          class="mt-4"
          type="warning"
          show-icon
          :message="$t('tg-card.paymentSettings.securityReminder')"
          :description="$t('tg-card.paymentSettings.securityDescription')"
        />
      </a-form>
      <div class="mt-4 flex justify-end"><a-button size="small" type="primary" :loading="saving" @click="saveSettings">{{$t('tg-card.paymentSettings.saveTron')}}</a-button></div>
    </a-card>
    <a-card :bordered="false" :loading="loading" :title="$t('tg-card.paymentSettings.epusdtTitle')">
      <template #extra><div class="flex items-center gap-2"><a-tag :color="settings.epusdtEnabled ? 'success' : 'default'">{{ settings.epusdtEnabled ? $t('tg-card.paymentSettings.enabled') : $t('tg-card.paymentSettings.disabled') }}</a-tag><a-switch v-model:checked="settings.epusdtEnabled" /></div></template>
      <a-form layout="vertical">
        <a-form-item :label="$t('tg-card.paymentSettings.epusdtBaseUrl')" required><a-input v-model:value="settings.epusdtBaseUrl" placeholder="https://pay.example.com" autocomplete="url"/></a-form-item>
        <a-form-item :label="$t('tg-card.paymentSettings.epusdtPid')" required><a-input v-model:value="settings.epusdtPid" autocomplete="off"/></a-form-item>
        <a-form-item :label="$t('tg-card.paymentSettings.epusdtSecret')"><a-input-password v-model:value="epusdtSecret" autocomplete="new-password" :placeholder="$t('tg-card.paymentSettings.epusdtSecretPlaceholder')"/><div class="mt-2 flex items-center gap-2"><a-tag :color="settings.epusdtSecretConfigured ? 'success' : 'warning'">{{ settings.epusdtSecretConfigured ? $t('tg-card.paymentSettings.secretConfigured') : $t('tg-card.paymentSettings.secretMissing') }}</a-tag><a-checkbox v-model:checked="clearEpusdtSecret">{{$t('tg-card.paymentSettings.clearSecret')}}</a-checkbox></div></a-form-item>
        <div class="mt-4 flex justify-end"><a-button type="primary" :loading="saving" @click="saveGateway('epusdt')">{{$t('tg-card.paymentSettings.saveEpusdt')}}</a-button></div>
      </a-form>
    </a-card>
    <a-card :bordered="false" :loading="loading" :title="$t('tg-card.paymentSettings.gmpayTitle')">
      <template #extra><div class="flex items-center gap-2"><a-tag :color="settings.gmpayEnabled ? 'success' : 'default'">{{ settings.gmpayEnabled ? $t('tg-card.paymentSettings.enabled') : $t('tg-card.paymentSettings.disabled') }}</a-tag><a-switch v-model:checked="settings.gmpayEnabled" /></div></template>
      <a-form layout="vertical">
        <a-form-item :label="$t('tg-card.paymentSettings.gmpayBaseUrl')" required><a-input v-model:value="settings.gmpayBaseUrl" placeholder="https://gmpay.example.com" autocomplete="url"/></a-form-item>
        <a-form-item :label="$t('tg-card.paymentSettings.gmpayPid')" required><a-input v-model:value="settings.gmpayPid" autocomplete="off"/></a-form-item>
        <a-form-item :label="$t('tg-card.paymentSettings.gmpaySecret')"><a-input-password v-model:value="gmpaySecret" autocomplete="new-password" :placeholder="$t('tg-card.paymentSettings.gmpaySecretPlaceholder')"/><div class="mt-2 flex items-center gap-2"><a-tag :color="settings.gmpaySecretConfigured ? 'success' : 'warning'">{{ settings.gmpaySecretConfigured ? $t('tg-card.paymentSettings.secretConfigured') : $t('tg-card.paymentSettings.secretMissing') }}</a-tag><a-checkbox v-model:checked="clearGmpaySecret">{{$t('tg-card.paymentSettings.clearSecret')}}</a-checkbox></div></a-form-item>
        <div class="mt-4 flex justify-end"><a-button type="primary" :loading="saving" @click="saveGateway('gmpay')">{{$t('tg-card.paymentSettings.saveGmpay')}}</a-button></div>
      </a-form>
    </a-card>
    <a-card :bordered="false" :title="$t('tg-card.paymentSettings.bonusTitle')">
      
      <div class="mb-4 flex flex-wrap items-center gap-3"><a-switch v-model:checked="bonusRule.enabled"/><span>{{$t('tg-card.paymentSettings.enableBonus')}}</span><a-button size="small" @click="addBonusTier">{{$t('tg-card.paymentSettings.addTier')}}</a-button></div>
      <div v-for="(tier,index) in bonusRule.tiers" :key="index" class="mb-3 grid grid-cols-1 gap-2 rounded-lg border border-gray-200 p-3 md:grid-cols-3">
       <a-form-item :label="$t('tg-card.paymentSettings.rechargeAtLeast')" class="mb-0"><a-input-number v-model:value="tier.minAmountAtomic" :min="1000000" :step="1000000" class="w-full" :formatter="(v:any)=>String(Number(v||0)/1000000)" :parser="(v:any)=>Number(v)*1000000"/></a-form-item>
       <a-form-item :label="$t('tg-card.paymentSettings.bonusType')" class="mb-0"><a-select v-model:value="tier.bonusType"><a-select-option value="fixed">{{$t('tg-card.paymentSettings.fixedBonus')}}</a-select-option><a-select-option value="percent">{{$t('tg-card.paymentSettings.percentBonus')}}</a-select-option></a-select></a-form-item>
       <a-form-item :label="tier.bonusType==='fixed'?$t('tg-card.paymentSettings.bonusAmount'):$t('tg-card.paymentSettings.bonusPercent')" class="mb-0"><a-input-number v-model:value="tier.bonusValue" :min="1" :max="tier.bonusType==='percent'?100:100000000" class="w-full" :formatter="(v:any)=>tier.bonusType==='fixed'?String(Number(v||0)/1000000):String(Number(v||0)/100)" :parser="(v:any)=>Math.round(Number(v)*(tier.bonusType==='fixed'?1000000:100))"/></a-form-item>
       <div class="md:col-span-3 flex justify-end"><a-button danger size="small" @click="removeBonusTier(index)">{{$t('tg-card.paymentSettings.deleteTier')}}</a-button></div>
      </div>
      <a-empty v-if="bonusRule.tiers.length===0" :description="$t('tg-card.paymentSettings.noTiers')"/>
      <div class="mt-4 flex justify-end"><a-button type="primary" :loading="bonusSaving" @click="saveBonusRule">{{$t('tg-card.paymentSettings.saveBonus')}}</a-button></div>
    </a-card>
  </div>
</template>
