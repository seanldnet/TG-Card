<script setup lang="ts">
import type { VbenFormSchema } from '#/adapter/form';

import { computed } from 'vue';

import { ProfilePasswordSetting, z } from '@vben/common-ui';
import { message } from 'ant-design-vue';

import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { useAuthStore } from '#/store';

const authStore = useAuthStore();

const formSchema = computed((): VbenFormSchema[] => [
  {
    fieldName: 'oldPassword',
    label: $t('tg-card.passwordSettings.currentPassword'),
    component: 'VbenInputPassword',
    componentProps: { placeholder: $t('tg-card.passwordSettings.currentPasswordPlaceholder') },
    rules: z.string().min(1, $t('tg-card.passwordSettings.currentPasswordPlaceholder')),
  },
  {
    fieldName: 'newPassword',
    label: $t('tg-card.passwordSettings.newPassword'),
    component: 'VbenInputPassword',
    componentProps: {
      passwordStrength: true,
      placeholder: $t('tg-card.passwordSettings.newPasswordPlaceholder'),
    },
    rules: z
      .string()
      .min(12, $t('tg-card.passwordSettings.newPasswordMin'))
      .max(256, $t('tg-card.passwordSettings.newPasswordMax')),
  },
  {
    fieldName: 'confirmPassword',
    label: $t('tg-card.passwordSettings.confirmPassword'),
    component: 'VbenInputPassword',
    componentProps: { placeholder: $t('tg-card.passwordSettings.confirmRequired') },
    dependencies: {
      rules(values) {
        return z
          .string()
          .min(1, $t('tg-card.passwordSettings.confirmRequired'))
          .refine((value) => value === values.newPassword, {
            message: $t('tg-card.passwordSettings.passwordMismatch'),
          });
      },
      triggerFields: ['newPassword'],
    },
  },
]);

async function handleSubmit(values: Record<string, string>) {
  await requestClient.post('/admin/password', {
    currentPassword: values.oldPassword,
    newPassword: values.newPassword,
  });
  message.success($t('tg-card.passwordSettings.passwordChanged'));
  await authStore.logout();
}
</script>

<template>
  <ProfilePasswordSetting
    class="w-full max-w-xl"
    :form-schema="formSchema"
    @submit="handleSubmit"
  />
</template>
