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
const CAP_WIDGET_VERSION = '0.1.63';
let capScriptPromise: Promise<void> | undefined;

function waitForCapWidget(timeoutMs = 8000): Promise<void> {
  const registry = (window as any).customElements as CustomElementRegistry | undefined;
  if (!registry) return Promise.reject(new Error('Custom Elements are not supported by this browser'));
  if (registry.get('cap-widget')) return Promise.resolve();
  return Promise.race([
    registry.whenDefined('cap-widget'),
    new Promise<void>((_, reject) => window.setTimeout(() => reject(new Error('Cap widget registration timed out')), timeoutMs)),
  ]);
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = src;
    script.async = true;
    script.dataset.tgCardCapWidget = 'true';
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`Cap widget script failed to load: ${src}`));
    document.head.appendChild(script);
  });
}

async function loadCapWidgetScript(domain: string) {
  if ((window as any).customElements?.get('cap-widget')) return;
  if (capScriptPromise) return capScriptPromise;

  const origin = new URL(domain).origin;
  capScriptPromise = (async () => {
    // Prefer capd's own asset server. This requires Instance settings >
    // Widget files > Serve widget files from this server to be enabled.
    try {
      (window as any).CAP_CUSTOM_WASM_URL = origin + '/assets/cap_wasm_bg.wasm';
      (window as any).CAP_CUSTOM_HASHWX_URL = origin + '/assets/hashwx.wasm';
      await loadScript(origin + '/assets/widget.js');
      await waitForCapWidget();
      return;
    } catch (selfHostedError) {
      // The asset server is optional in capd. Fall back to a pinned official
      // widget release rather than leaving the login page with a broken widget.
      delete (window as any).CAP_CUSTOM_WASM_URL;
      delete (window as any).CAP_CUSTOM_HASHWX_URL;
      try {
        await loadScript(`https://cdn.jsdelivr.net/npm/cap-widget@${CAP_WIDGET_VERSION}`);
        await waitForCapWidget();
      } catch (cdnError) {
        throw new Error(`Cap widget unavailable. Self-hosted asset error: ${String(selfHostedError)}. CDN error: ${String(cdnError)}`);
      }
    }
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
      await loadCapWidgetScript(capDomain.value);
      await nextTick();
      mountCapWidget();
    }
  } catch (error) {
    // Fail closed: the backend may still require a token even when the
    // public configuration or widget cannot be loaded.
    capEnabled.value = true;
    capError.value = `人机验证初始化失败：${error instanceof Error ? error.message : String(error)}。请联系管理员检查 Cap 服务及 Widget files 配置。`;
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
