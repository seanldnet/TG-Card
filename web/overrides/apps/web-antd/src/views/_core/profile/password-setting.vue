<script setup lang="ts">
import type { VbenFormSchema } from '#/adapter/form';

import { computed } from 'vue';

import { ProfilePasswordSetting, z } from '@vben/common-ui';
import { message } from 'ant-design-vue';

import { requestClient } from '#/api/request';
import { useAuthStore } from '#/store';

const authStore = useAuthStore();

const formSchema = computed((): VbenFormSchema[] => [
  {
    fieldName: 'oldPassword',
    label: '当前密码',
    component: 'VbenInputPassword',
    componentProps: { placeholder: '请输入当前密码' },
    rules: z.string().min(1, '请输入当前密码'),
  },
  {
    fieldName: 'newPassword',
    label: '新密码',
    component: 'VbenInputPassword',
    componentProps: {
      passwordStrength: true,
      placeholder: '至少 12 位，建议使用大小写字母、数字和符号',
    },
    rules: z
      .string()
      .min(12, '新密码至少需要 12 位')
      .max(256, '新密码不能超过 256 位'),
  },
  {
    fieldName: 'confirmPassword',
    label: '确认新密码',
    component: 'VbenInputPassword',
    componentProps: { placeholder: '请再次输入新密码' },
    dependencies: {
      rules(values) {
        return z
          .string()
          .min(1, '请再次输入新密码')
          .refine((value) => value === values.newPassword, {
            message: '两次输入的密码不一致',
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
  message.success('密码修改成功，请使用新密码重新登录');
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
