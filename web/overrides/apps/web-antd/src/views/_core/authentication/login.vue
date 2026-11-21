<script lang="ts" setup>
import type { VbenFormSchema } from '@vben/common-ui';

import { computed, markRaw, onMounted, ref } from 'vue';

import { AuthenticationLogin, z } from '@vben/common-ui';
import { $t } from '@vben/locales';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

import { useAuthStore } from '#/store';
import CapWidgetField from './cap-widget-field.vue';

defineOptions({ name: 'Login' });
const authStore = useAuthStore();
const capEnabled = ref(false);
const capDomain = ref('');
const capSiteKey = ref('');
const capToken = ref('');
const capError = ref('');
const capConfigLoaded = ref(false);
const capWidgetReady = ref(false);
const capWidgetNonce = ref(0);
const totpRequired = ref(false);
const recoveryMode = ref(false);
const CAP_WIDGET_VERSION = '0.1.63';
let capScriptPromise: Promise<void> | undefined;

function waitForCapWidget(timeoutMs = 8000): Promise<void> {
  const registry = (window as any).customElements as CustomElementRegistry | undefined;
  if (!registry) return Promise.reject(new Error('Custom Elements are not supported by this browser'));
  if (registry.get('cap-widget')) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const timeout = window.setTimeout(() => reject(new Error('Cap widget registration timed out')), timeoutMs);
    registry.whenDefined('cap-widget').then(
      () => {
        window.clearTimeout(timeout);
        resolve();
      },
      (error) => {
        window.clearTimeout(timeout);
        reject(error);
      },
    );
  });
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    const timeout = window.setTimeout(() => {
      script.remove();
      reject(new Error('Cap Widget CDN script loading timed out'));
    }, 12_000);
    script.src = src;
    script.async = true;
    script.dataset.tgCardCapWidget = 'true';
    script.onload = () => {
      window.clearTimeout(timeout);
      resolve();
    };
    script.onerror = () => {
      window.clearTimeout(timeout);
      script.remove();
      reject(new Error('Cap Widget CDN script failed to load'));
    };
    document.head.appendChild(script);
  });
}

async function loadCapWidgetScript() {
  if ((window as any).customElements?.get('cap-widget')) return;
  if (capScriptPromise) return capScriptPromise;

  capScriptPromise = (async () => {
    // Current Cap versions serve the widget through jsDelivr by default.
    // The capd instance remains the challenge/siteverify API endpoint.
    await loadScript(`https://cdn.jsdelivr.net/npm/cap-widget@${CAP_WIDGET_VERSION}`);
    await waitForCapWidget();
  })().catch((error) => {
    capScriptPromise = undefined;
    throw error;
  });
  return capScriptPromise;
}

async function loadCapConfig() {
  try {
    const config = await requestClient.get<{ enabled: boolean; domain: string; siteKey: string }>('/auth/cap-config');
    capEnabled.value = !!config.enabled;
    capDomain.value = config.domain || '';
    capSiteKey.value = config.siteKey || '';
    if (capEnabled.value) {
      if (!capDomain.value || !capSiteKey.value) throw new Error('Cap configuration is incomplete');
      await loadCapWidgetScript();
      capWidgetReady.value = true;
    }
  } catch (error) {
    // Fail closed: the backend may still require a token even when the
    // public configuration or widget cannot be loaded.
    capEnabled.value = true;
    capError.value = `人机验证初始化失败：${error instanceof Error ? error.message : String(error)}。请检查浏览器能否访问 cdn.jsdelivr.net，以及 Cap 服务地址和 Site Key 是否正确。`;
    message.error(capError.value);
  } finally {
    capConfigLoaded.value = true;
  }
}

async function handleLogin(data: Record<string, any>) {
  if (!capConfigLoaded.value) {
    message.warning('人机验证配置仍在加载，请稍后重试');
    return;
  }
  if (capEnabled.value && !capToken.value) {
    message.warning(capError.value || '请先完成人机验证');
    return;
  }

  // Only reveal the second-factor field after the username/password pair is
  // verified. This avoids exposing a permanent TOTP field to accounts that
  // have not enabled two-factor authentication.
  let requirement: { enabled: boolean };
  try {
    requirement = await requestClient.post<{ enabled: boolean }>('/auth/totp-required', {
      username: data.username,
      password: data.password,
    });
  } catch {
    return;
  }
  totpRequired.value = requirement.enabled;
  if (!requirement.enabled) recoveryMode.value = false;

  const secondFactor = recoveryMode.value ? data.totpRecoveryCode : data.totpCode;
  if (requirement.enabled && !String(secondFactor || '').trim()) {
    message.info('已开启双重验证，请输入 6 位动态验证码');
    return;
  }

  try {
    await authStore.authLogin({
      ...data,
      ...(capEnabled.value ? { capToken: capToken.value } : {}),
      ...(recoveryMode.value
        ? { totpCode: undefined, totpRecoveryCode: data.totpRecoveryCode }
        : { totpRecoveryCode: undefined }),
    } as any);
  } finally {
    if (capEnabled.value) {
      capToken.value = '';
      capWidgetNonce.value += 1;
    }
  }
}

onMounted(loadCapConfig);

const formSchema = computed((): VbenFormSchema[] => [
  {
    component: 'VbenInput',
    componentProps: { placeholder: $t('authentication.usernameTip') },
    fieldName: 'username',
    label: $t('authentication.username'),
    rules: z.string().min(1, { message: $t('authentication.usernameTip') }),
  },
  {
    component: 'VbenInputPassword',
    componentProps: { placeholder: $t('authentication.password') },
    fieldName: 'password',
    label: $t('authentication.password'),
    rules: z.string().min(1, { message: $t('authentication.passwordTip') }),
  },
  ...(totpRequired.value
    ? [{
        component: 'VbenInput',
        componentProps: {
          placeholder: recoveryMode.value
            ? '请输入未使用的一次性恢复码'
            : '已开启双重验证，请输入 6 位动态验证码',
        },
        fieldName: recoveryMode.value ? 'totpRecoveryCode' : 'totpCode',
        label: recoveryMode.value ? '一次性恢复码' : '动态验证码',
        rules: z.string().optional(),
      }]
    : []),
  ...(capEnabled.value
    ? [{
        component: markRaw(CapWidgetField),
        componentProps: {
          apiEndpoint: `${capDomain.value.replace(/\/+$/, '')}/${encodeURIComponent(capSiteKey.value)}/`,
          error: capError.value,
          nonce: capWidgetNonce.value,
          ready: capWidgetReady.value,
          onTokenChange: (token: string) => {
            capToken.value = token;
            if (token) capError.value = '';
          },
          onWidgetError: (error: string) => {
            capToken.value = '';
            capError.value = error;
          },
        },
        fieldName: 'capVerification',
        label: '安全验证',
        rules: z.boolean().optional().default(false),
      }]
    : []),
]);
</script>

<template>
  <div class="w-full">
    <AuthenticationLogin
      :form-schema="formSchema"
      :loading="authStore.loginLoading"
      :show-code-login="false"
      :show-qrcode-login="false"
      :show-third-party-login="false"
      @submit="handleLogin"
    />
    <div v-if="totpRequired" class="mt-3 text-center">
      <button
        type="button"
        class="text-sm text-primary hover:underline"
        @click="recoveryMode = !recoveryMode"
      >
        {{ recoveryMode ? '返回输入动态验证码' : '无法使用验证器？使用一次性恢复码' }}
      </button>
    </div>
  </div>
</template>
