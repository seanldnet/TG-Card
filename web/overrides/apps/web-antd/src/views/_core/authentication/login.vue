<script lang="ts" setup>
import type { VbenFormSchema } from '@vben/common-ui';

import { computed, nextTick, onMounted, ref } from 'vue';

import { AuthenticationLogin, z } from '@vben/common-ui';
import { $t } from '@vben/locales';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

import { useAuthStore } from '#/store';

defineOptions({ name: 'Login' });
const authStore = useAuthStore();
const capEnabled = ref(false);
const capDomain = ref('');
const capSiteKey = ref('');
const capToken = ref('');
const capError = ref('');
const capWidgetContainer = ref<HTMLElement>();
let capScriptPromise: Promise<void> | undefined;

async function loadCapWidgetScript(domain: string) {
  const origin = new URL(domain).origin;
  const src = origin + '/assets/widget.js';
  if ((window as any).customElements?.get('cap-widget')) return;
  if (!capScriptPromise) {
    capScriptPromise = new Promise<void>((resolve, reject) => {
      const existing = document.querySelector<HTMLScriptElement>('script[data-tg-card-cap-widget]');
      if (existing) {
        existing.addEventListener('load', () => resolve(), { once: true });
        existing.addEventListener('error', () => reject(new Error('Cap widget script failed to load')), { once: true });
        return;
      }
      const script = document.createElement('script');
      script.src = src;
      script.async = true;
      script.dataset.tgCardCapWidget = 'true';
      script.onload = () => resolve();
      script.onerror = () => reject(new Error('Cap widget script failed to load'));
      document.head.appendChild(script);
    });
  }
  await capScriptPromise;
  await (window as any).customElements?.whenDefined('cap-widget');
}

function mountCapWidget() {
  if (!capEnabled.value || !capWidgetContainer.value) return;
  const widget = document.createElement('cap-widget') as HTMLElement & { reset?: () => void };
  widget.setAttribute('data-cap-api-endpoint', new URL(capDomain.value).origin + '/' + encodeURIComponent(capSiteKey.value) + '/');
  widget.setAttribute('data-cap-lang', 'zh');
  widget.addEventListener('solve', (event) => {
    capToken.value = (event as CustomEvent<{ token: string }>).detail?.token || '';
    capError.value = '';
  });
  widget.addEventListener('error', (event) => {
    capToken.value = '';
    capError.value = (event as CustomEvent<{ message?: string }>).detail?.message || '人机验证加载失败，请刷新页面重试';
  });
  capWidgetContainer.value.replaceChildren(widget);
}

async function loadCapConfig() {
  try {
    const config = await requestClient.get<{ enabled: boolean; domain: string; siteKey: string }>('/auth/cap-config');
    capEnabled.value = !!config.enabled;
    capDomain.value = config.domain || '';
    capSiteKey.value = config.siteKey || '';
    if (capEnabled.value) {
      if (!capDomain.value || !capSiteKey.value) throw new Error('Cap configuration is incomplete');
      await loadCapWidgetScript(capDomain.value);
      await nextTick();
      mountCapWidget();
    }
  } catch {
    capEnabled.value = false;
    capError.value = '人机验证配置暂时无法读取；如果登录被拒绝，请联系管理员检查 Cap 服务。';
    message.error(capError.value);
  }
}

async function handleLogin(data: Record<string, any>) {
  if (capEnabled.value && !capToken.value) {
    message.warning(capError.value || '请先完成人机验证');
    return;
  }
  try {
    await authStore.authLogin({ ...data, ...(capEnabled.value ? { capToken: capToken.value } : {}) } as any);
  } finally {
    if (capEnabled.value) {
      capToken.value = '';
      capWidgetContainer.value?.replaceChildren();
      mountCapWidget();
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
  <div v-if="capEnabled" class="mx-auto mt-4 flex w-full max-w-sm flex-col items-center gap-2">
    <div ref="capWidgetContainer" class="min-h-10" />
    <p v-if="capError" class="text-xs text-red-500">{{ capError }}</p>
  </div>
  <p v-else-if="capError" class="mx-auto mt-3 max-w-sm text-center text-xs text-amber-600">{{ capError }}</p>
  </div>
</template>
