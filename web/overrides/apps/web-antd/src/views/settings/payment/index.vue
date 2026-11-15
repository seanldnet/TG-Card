<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface PaymentSettings {
  apiKeyConfigured: boolean;
  receiveAddress: string;
  usdtContract: string;
  enabled: boolean;
}

const loading = ref(false);
const saving = ref(false);
const apiKey = ref('');
const clearApiKey = ref(false);
const settings = reactive<PaymentSettings>({
  apiKeyConfigured: false,
  receiveAddress: '',
  usdtContract: 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
  enabled: false,
});

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

onMounted(loadSettings);
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
        <a-space>
          <a-button type="primary" :loading="saving" @click="saveSettings">保存并应用</a-button>
          <a-button :disabled="saving" @click="loadSettings">重新加载</a-button>
        </a-space>
      </a-form>
    </a-card>
  </div>
</template>
