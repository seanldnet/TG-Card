<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface BonusTier { minAmountAtomic: number; bonusType: 'fixed' | 'percent'; bonusValue: number; }
interface BonusRule { enabled: boolean; tiers: BonusTier[]; }

interface PaymentSettings {
  apiKeyConfigured: boolean;
  receiveAddress: string;
  usdtContract: string;
  enabled: boolean;
}

const loading = ref(false);
const bonusSaving = ref(false);
const bonusRule = reactive<BonusRule>({enabled:false,tiers:[]});
const saving = ref(false);
const apiKey = ref('');
const clearApiKey = ref(false);
const settings = reactive<PaymentSettings>({
  apiKeyConfigured: false,
  receiveAddress: '',
  usdtContract: 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
  enabled: false,
});

async function loadBonusRule(){
  try { const data=await requestClient.get<BonusRule>('/settings/recharge-bonus'); Object.assign(bonusRule,data); bonusRule.tiers=data.tiers||[]; }
  catch { message.error('充值赠送规则加载失败'); }
}
async function saveBonusRule(){
  bonusSaving.value=true;
  try { const data=await requestClient.put<BonusRule>('/settings/recharge-bonus',bonusRule); Object.assign(bonusRule,data); bonusRule.tiers=data.tiers||[]; message.success('充值赠送规则已保存'); }
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

async function saveSettings() {
  saving.value = true;
  try {
    const data = await requestClient.put<PaymentSettings>('/settings/payment', {
      apiKey: apiKey.value,
      clearApiKey: clearApiKey.value,
      receiveAddress: settings.receiveAddress,
      usdtContract: settings.usdtContract,
      enabled: settings.enabled,
    });
    Object.assign(settings, data);
    apiKey.value = '';
    clearApiKey.value = false;
    message.success('支付设置已保存，监听器已实时更新，无需重启服务');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存失败，请检查配置');
  } finally {
    saving.value = false;
  }
}

onMounted(()=>{void loadSettings();void loadBonusRule()});
</script>

<template>
  <div class="mx-auto w-full min-w-0 max-w-5xl p-3 md:p-6">
    <a-card :bordered="false" :loading="loading" title="USDT - TRC20 收款配置">
      <template #extra>
        <a-tag :color="settings.enabled ? 'success' : 'default'">
          {{ settings.enabled ? '收款已启用' : '收款未启用' }}
        </a-tag>
      </template>
      <a-alert
        class="mb-5"
        type="info"
        show-icon
        message="在此页面直接配置收款信息，无需修改服务器 .env 或重启 Docker。API Key 使用 AES-256-GCM 加密后保存在当前数据库中。"
      />
      <a-form layout="vertical">
        <a-form-item label="TronGrid API Key" extra="密钥不会回显。留空表示保持现有密钥；如需更换，请输入新密钥。">
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
        <a-form-item label="TRON 收款地址" required extra="请输入商户控制的 TRON 主网地址，系统会校验 Base58Check 格式。">
          <a-input
            v-model:value="settings.receiveAddress"
            autocomplete="off"
            placeholder="T 开头的 TRON 主网地址"
          />
        </a-form-item>
        <a-form-item label="USDT TRC20 合约地址" extra="默认是 TRON 主网官方 USDT 合约地址。">
          <a-input v-model:value="settings.usdtContract" autocomplete="off" />
        </a-form-item>
        <a-divider />
        <a-form-item label="启用 TRC20 自动收款">
          <div class="flex items-center gap-3">
            <a-switch v-model:checked="settings.enabled" />
            <span class="text-sm text-gray-500">
              开启后 Bot 才会显示本订单专属应付金额和收款地址，并启动链上到账监听。
            </span>
          </div>
        </a-form-item>
        <a-alert
          class="mb-4"
          type="warning"
          show-icon
          message="安全提醒"
          description="请只填写商户自己的收款地址。系统仅监听到账，不需要钱包私钥；少付、多付、过期转账仍需人工对账。"
        />
        <div class="mt-7 border-t border-gray-100 pt-5">
          <a-space wrap>
            <a-button type="primary" :loading="saving" @click="saveSettings">保存并应用</a-button>
            <a-button :disabled="saving" @click="loadSettings">重新加载</a-button>
          </a-space>
        </div>
      </a-form>
    </a-card>
    <a-card class="mt-6 md:mt-8" :bordered="false" title="充值赠送规则">
      <a-alert class="mb-4" type="info" show-icon message="规则按充值本金匹配最高门槛；固定赠送填写 USDT 金额，百分比赠送填写百分数。创建充值单时会锁定赠送金额，支付确认时与本金在同一事务中入账，重复回调不会重复赠送。"/>
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
