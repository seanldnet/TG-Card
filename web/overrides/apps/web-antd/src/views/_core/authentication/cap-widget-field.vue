<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';

const props = withDefaults(defineProps<{
  apiEndpoint: string;
  error?: string;
  nonce: number;
  ready: boolean;
}>(), { error: '' });

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  tokenChange: [token: string];
  widgetError: [message: string];
}>();

const container = ref<HTMLElement>();
let widget: (HTMLElement & { reset?: () => void }) | undefined;

function mountWidget() {
  if (!props.ready || !container.value) return;
  widget?.remove();
  widget = document.createElement('cap-widget') as HTMLElement & { reset?: () => void };
  widget.setAttribute('data-cap-api-endpoint', props.apiEndpoint);
  widget.setAttribute('data-cap-lang', 'zh');
  widget.setAttribute('data-cap-i18n-initial-state', '点击进行安全验证');
  widget.setAttribute('data-cap-i18n-verifying-label', '正在验证...');
  widget.setAttribute('data-cap-i18n-solved-label', '验证成功');
  widget.setAttribute('data-cap-i18n-error-label', '验证失败');
  widget.setAttribute('data-cap-i18n-required-label', '请先完成人机验证');
  widget.setAttribute('data-cap-i18n-verify-aria-label', '点击进行安全验证');
  widget.setAttribute('data-cap-i18n-verifying-aria-label', '正在验证，请稍候');
  widget.setAttribute('data-cap-i18n-verified-aria-label', '已完成安全验证');
  widget.setAttribute('data-cap-i18n-error-aria-label', '验证失败，请重试');
  widget.style.setProperty('--cap-widget-width', '100%');
  widget.addEventListener('solve', (event) => {
    const token = (event as CustomEvent<{ token?: string }>).detail?.token || '';
    emit('tokenChange', token);
    emit('update:modelValue', !!token);
  });
  widget.addEventListener('error', (event) => {
    emit('tokenChange', '');
    emit('update:modelValue', false);
    const detail = (event as CustomEvent<{ code?: string; message?: string }>).detail;
    emit('widgetError', detail?.message
      ? `人机验证失败（${detail.code || 'unknown'}）：${detail.message}`
      : '人机验证失败，请重试');
  });
  container.value.replaceChildren(widget);
}

watch(() => [props.ready, props.nonce, props.apiEndpoint] as const, async () => {
  await nextTick();
  mountWidget();
});

onMounted(mountWidget);
onBeforeUnmount(() => widget?.remove());
</script>

<template>
  <div class="w-full min-w-0">
    <div class="mb-2 text-sm font-medium text-foreground">安全验证</div>
    <div
      ref="container"
      class="cap-widget-field flex min-h-[58px] w-full items-center"
    />
    <p v-if="error" class="mt-1 text-xs text-red-500">{{ error }}</p>
    <p v-else-if="!ready" class="mt-1 text-xs text-red-500">人机验证暂不可用，请刷新页面或联系管理员</p>
  </div>
</template>

<style scoped>
.cap-widget-field :deep(cap-widget) {
  display: block;
  width: 100%;
  max-width: 100%;
}
</style>
