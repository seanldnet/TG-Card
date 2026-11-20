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
const capConfigLoaded = ref(false);
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
    const detail = (event as CustomEvent<{ code?: string; message?: string }>).detail;
    capError.value = detail?.message ? `人机验证失败（${detail.code || 'unknown'}）：${detail.message}` : '人机验证加载失败，请刷新页面重试';
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
      await loadCapWidgetScript();
      await nextTick();
      mountCapWidget();
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
