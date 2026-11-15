<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';
import { VbenAvatar, VbenButton } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';
import { useVbenModal } from '@vben/common-ui';

import { useVbenForm } from '#/adapter/form';
import { requestClient } from '#/api/request';
import { useAuthStore } from '#/store';

interface AdminProfile {
  id: string;
  username: string;
  realName: string;
  avatar: string;
  roles: string[];
}
const loading = ref(false);
const saving = ref(false);
const uploading = ref(false);
const profile = reactive<AdminProfile>({
  id: '',
  username: '',
  realName: '',
  avatar: '',
  roles: [],
});
const userStore = useUserStore();
const authStore = useAuthStore();
const passwordModalOpen = ref(false);
const passwordSaving = ref(false);
const [PasswordModal, passwordModalApi] = useVbenModal({
  title: '修改登录密码',
  onConfirm: submitPassword,
});
const [ProfileForm, profileFormApi] = useVbenForm({
  commonConfig: {
    componentProps: { class: 'w-full' },
  },
  layout: 'vertical',
  schema: [
    {
      component: 'Input',
      fieldName: 'realName',
      label: '昵称',
      componentProps: {
        maxlength: 80,
        placeholder: '请输入显示昵称',
      },
    },
  ],
  showDefaultActions: false,
  wrapperClass: 'grid-cols-1',
});
const [AvatarForm] = useVbenForm({
  commonConfig: {
    componentProps: { class: 'w-full' },
  },
  layout: 'vertical',
  schema: [
    {
      component: 'Upload',
      fieldName: 'avatarFile',
      label: '头像上传',
      componentProps: {
        accept: 'image/png,image/jpeg,image/webp',
        listType: 'picture',
        maxCount: 1,
        maxSize: 2,
        showUploadList: false,
        beforeUpload: uploadAvatar,
      },
    },
  ],
  showDefaultActions: false,
  wrapperClass: 'grid-cols-1',
});
const [PasswordForm, passwordFormApi] = useVbenForm({
  commonConfig: {
    componentProps: { class: 'w-full' },
  },
  layout: 'vertical',
  schema: [
    {
      component: 'InputPassword',
      fieldName: 'currentPassword',
      label: '当前密码',
      componentProps: { autocomplete: 'current-password', placeholder: '请输入当前密码' },
    },
    {
      component: 'InputPassword',
      fieldName: 'newPassword',
      label: '新密码',
      componentProps: { autocomplete: 'new-password', placeholder: '至少 12 位' },
    },
    {
      component: 'InputPassword',
      fieldName: 'confirmPassword',
      label: '确认新密码',
      componentProps: { autocomplete: 'new-password', placeholder: '再次输入新密码' },
    },
  ],
  showDefaultActions: false,
  wrapperClass: 'grid-cols-1',
});

async function loadProfile() {
  loading.value = true;
  try {
    const data = await requestClient.get<AdminProfile>('/user/info');
    Object.assign(profile, data);
    await profileFormApi.setValues({ realName: data.realName || '' });
    userStore.setUserInfo(data as any);
  } catch {
    message.error('个人资料加载失败');
  } finally {
    loading.value = false;
  }
}

async function saveProfile() {
  const { valid } = await profileFormApi.validate();
  if (!valid) return;
  const values = await profileFormApi.getValues();
  saving.value = true;
  try {
    const result = await requestClient.put<{ realName: string; avatar: string }>('/user/profile', {
      realName: String(values.realName || '').trim(),
      avatar: profile.avatar,
    });
    profile.realName = result.realName;
    profile.avatar = result.avatar;
    const updated = await requestClient.get<AdminProfile>('/user/info');
    Object.assign(profile, updated);
    userStore.setUserInfo(updated as any);
    message.success('个人资料已保存');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '保存个人资料失败');
  } finally {
    saving.value = false;
  }
}

async function uploadAvatar(file: any) {
  const imageFile = file?.originFileObj || file;
  if (!(imageFile instanceof Blob)) {
    message.error('无法读取所选头像文件');
    return false;
  }
  if (imageFile.size > 2 * 1024 * 1024) {
    message.warning('头像文件不能超过 2 MB');
    return false;
  }
  uploading.value = true;
  try {
    const body = new FormData();
    body.append('file', imageFile, imageFile.name || 'avatar');
    const result = await requestClient.post<{ avatar: string }>('/user/avatar', body);
    profile.avatar = result.avatar;
    message.success('头像已上传，请保存个人资料以应用');
  } catch (error: any) {
    message.error(error?.response?.data?.message || '头像上传失败');
  } finally {
    uploading.value = false;
  }
  // The avatar is sent explicitly to the TG-Card endpoint above, not to Ant Upload.
  return false;
}

function openPasswordModal() {
  void passwordFormApi.setValues({
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
  });
  passwordModalApi.open();
}

async function submitPassword() {
  const { valid } = await passwordFormApi.validate();
  if (!valid) return;
  const values = await passwordFormApi.getValues();
  if (String(values.newPassword || '').length < 12) {
    message.warning('新密码至少需要 12 位');
    return;
  }
  if (values.newPassword !== values.confirmPassword) {
    message.warning('两次输入的新密码不一致');
    return;
  }
  passwordSaving.value = true;
  try {
    await requestClient.post('/admin/password', {
      currentPassword: values.currentPassword,
      newPassword: values.newPassword,
    });
    passwordModalApi.close();
    message.success('密码修改成功，请使用新密码重新登录');
    await authStore.logout();
  } catch (error: any) {
    message.error(error?.response?.data?.message || '密码修改失败');
  } finally {
    passwordSaving.value = false;
  }
}

onMounted(loadProfile);
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-4xl flex-col gap-4 p-3 md:p-6">
    <a-card :bordered="false" :loading="loading">
      <div class="flex min-w-0 flex-col items-center gap-3 sm:flex-row sm:items-center">
        <VbenAvatar :src="profile.avatar || undefined" class="size-20 shrink-0" />
        <div class="min-w-0 flex-1 text-center sm:text-left">
          <div class="break-all text-lg font-semibold">{{ profile.realName || profile.username || '管理员' }}</div>
          <div class="break-all text-sm text-gray-500">{{ profile.username }}</div>
          <a-space class="mt-2" wrap>
            <a-tag v-for="role in profile.roles" :key="role" color="blue">{{ role }}</a-tag>
            <a-tag color="success">账户正常</a-tag>
          </a-space>
        </div>
        <div class="w-full min-w-0 sm:w-auto">
          <AvatarForm />
          <a-spin v-if="uploading" size="small" />
        </div>
      </div>
    </a-card>

    <a-card :bordered="false" title="个人资料">
      <ProfileForm />
      <div class="mt-4 flex justify-end">
        <VbenButton type="primary" :loading="saving" @click="saveProfile">保存资料</VbenButton>
      </div>
    </a-card>

    <a-card :bordered="false" title="账户安全">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <div class="font-medium">登录密码</div>
          <div class="mt-1 text-sm text-gray-500">定期更换密码，保护后台账户安全</div>
        </div>
        <VbenButton class="w-full sm:w-auto" @click="openPasswordModal">修改密码</VbenButton>
      </div>
    </a-card>

    <PasswordModal class="w-full max-w-lg" :confirm-loading="passwordSaving">
      <div class="p-4 sm:p-6">
        <PasswordForm />
      </div>
    </PasswordModal>
  </div>
</template>
