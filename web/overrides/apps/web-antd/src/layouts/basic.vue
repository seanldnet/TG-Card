<script lang="ts" setup>
import type { NotificationItem } from '@vben/layouts';

import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import { AuthenticationLoginExpiredModal } from '@vben/common-ui';
import { useWatermark } from '@vben/hooks';
import {
  BasicLayout,
  LockScreen,
  Notification,
  UserDropdown,
} from '@vben/layouts';
import { preferences, updatePreferences, usePreferences } from '@vben/preferences';
import { useAccessStore, useUserStore } from '@vben/stores';
import { useWindowSize } from '@vueuse/core';

import { $t } from '#/locales';
import { useAuthStore } from '#/store';
import { requestClient } from '#/api/request';
import LoginForm from '#/views/_core/authentication/login.vue';

const notifications = ref<NotificationItem[]>([]);
const router = useRouter();
const userStore = useUserStore();
const authStore = useAuthStore();
const accessStore = useAccessStore();
const { destroyWatermark, updateWatermark } = useWatermark();
const { isDark } = usePreferences();
const { width } = useWindowSize();
const showDot = computed(() => notifications.value.some((item) => !item.isRead));

// Keep the navigation drawer available after route changes, including the profile page.
watch(
  () => router.currentRoute.value.fullPath,
  () => updatePreferences({ sidebar: { enable: true, hidden: false } }),
);

// Preserve the user's desktop widget preferences and switch the official Vben
// layout to a compact, mobile-first header without touching the drawer toggle.
const desktopWidgetSettings = { ...preferences.widget };
const desktopPreferencesEnabled = preferences.app.enablePreferences;
watch(
  width,
  (viewportWidth) => {
    const mobile = viewportWidth < 768;
    updatePreferences({
      app: {
        isMobile: mobile,
        enablePreferences: mobile ? false : desktopPreferencesEnabled,
      },
      header: { enable: true, hidden: false, mode: 'fixed' },
      sidebar: { enable: true, hidden: false },
      widget: mobile
        ? {
            sidebarToggle: true,
            globalSearch: false,
            languageToggle: false,
            timezone: false,
            fullscreen: false,
            refresh: false,
            notification: false,
            lockScreen: false,
            themeToggle: false,
            logoutButtonPosition: 'user-dropdown',
          }
        : desktopWidgetSettings,
    });
  },
  { immediate: true },
);

// Use the site name maintained by the administrator as the Vben application title.
onMounted(async () => {
  try {
    const settings = await requestClient.get<{ siteName?: string }>('/settings/site');
    if (settings.siteName?.trim()) updatePreferences({ app: { name: settings.siteName.trim() } });
  } catch {
    // Keep the shell available when optional site settings cannot be read.
  }
});

const menus = computed(() => [
  {
    handler: () => router.push({ name: 'Profile' }),
    icon: 'lucide:user-round',
    text: $t('tg-card.navigation.profile'),
  },
]);

const avatar = computed(
  () => userStore.userInfo?.avatar || preferences.app.defaultAvatar,
);

async function handleLogout() {
  await authStore.logout(false);
}
function handleNoticeClear() {
  notifications.value = [];
}
function markRead(id: number | string) {
  const item = notifications.value.find((item) => item.id === id);
  if (item) item.isRead = true;
}
function remove(id: number | string) {
  notifications.value = notifications.value.filter((item) => item.id !== id);
}
function handleMakeAll() {
  notifications.value.forEach((item) => (item.isRead = true));
}
const viewAll = () => {};
function handleClick(item: NotificationItem) {
  if (!item.link) return;
  if (item.link.startsWith('http://') || item.link.startsWith('https://')) {
    window.open(item.link, '_blank', 'noopener,noreferrer');
  } else {
    router.push({ path: item.link, query: item.query || {}, state: item.state });
  }
}

watch(
  () => ({
    enable: preferences.app.watermark,
    content: preferences.app.watermarkContent,
    isDark: isDark.value,
  }),
  async ({ enable, content, isDark: dark }) => {
    if (!enable) {
      destroyWatermark();
      return;
    }
    const watermarkColor = dark ? 'rgba(255,255,255,.12)' : 'rgba(0,0,0,.12)';
    await updateWatermark({
      advancedStyle: {
        colorStops: [
          { color: watermarkColor, offset: 0 },
          { color: watermarkColor, offset: 1 },
        ],
        type: 'linear',
      },
      content: content || `${userStore.userInfo?.username} - ${userStore.userInfo?.realName}`,
    });
  },
  { immediate: true },
);
</script>

<template>
  <BasicLayout
    :avatar="avatar"
    :text="userStore.userInfo?.realName || userStore.userInfo?.username"
    @clear-preferences-and-logout="handleLogout"
    @logout="handleLogout"
  >
    <template #user-dropdown>
      <UserDropdown
        :avatar="avatar"
        :menus="menus"
        :text="userStore.userInfo?.realName || userStore.userInfo?.username"
        :description="$t('tg-card.navigation.accountManagement')"
        @clear-preferences-and-logout="handleLogout"
        @logout="handleLogout"
      />
    </template>
    <template #notification>
      <Notification
        :dot="showDot"
        :notifications="notifications"
        @clear="handleNoticeClear"
        @read="(item) => item.id && markRead(item.id)"
        @remove="(item) => item.id && remove(item.id)"
        @make-all="handleMakeAll"
        @on-click="handleClick"
        @view-all="viewAll"
      />
    </template>
    <template #extra>
      <AuthenticationLoginExpiredModal v-model:open="accessStore.loginExpired" :avatar="avatar">
        <LoginForm />
      </AuthenticationLoginExpiredModal>
    </template>
    <template #lock-screen>
      <LockScreen :avatar="avatar" @to-login="handleLogout" />
    </template>
  </BasicLayout>
</template>
