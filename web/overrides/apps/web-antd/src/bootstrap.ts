import { createApp, watchEffect } from 'vue';

import { registerAccessDirective } from '@vben/access';
import { registerLoadingDirective } from '@vben/common-ui/es/loading';
import { preferences, updatePreferences } from '@vben/preferences';
import { initStores } from '@vben/stores';
import '@vben/styles';
import '@vben/styles/antd';
import './styles/mobile.css';

import Antd from 'ant-design-vue';
import { useTitle } from '@vueuse/core';

import { $t, setupI18n } from '#/locales';

import { initComponentAdapter } from './adapter/component';
import { initSetupVbenForm } from './adapter/form';
import App from './app.vue';
import { router } from './router';

async function bootstrap(namespace: string) {
  // Vben 5.8.0 does not infer mobile layout from viewport width.
  // Keep its official mobile drawer mode synchronized with the device.
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    const mobileQuery = window.matchMedia('(max-width: 767px)');
    const syncMobileLayout = () => {
      updatePreferences({ app: { isMobile: mobileQuery.matches } });
    };
    syncMobileLayout();
    mobileQuery.addEventListener('change', syncMobileLayout);
  }

  await initComponentAdapter();
  await initSetupVbenForm();

  const app = createApp(App);

  // TG-Card business views use Ant Design Vue tags (a-card, a-form,
  // a-input, a-table, etc.). Register the official component library
  // globally so these tags resolve to Vue components instead of unknown
  // HTML elements that render as blank/plain text.
  app.use(Antd);

  registerLoadingDirective(app, {
    loading: 'loading',
    spinning: 'spinning',
  });

  await setupI18n(app);
  await initStores(app, { namespace });
  registerAccessDirective(app);

  const { initTippy } = await import('@vben/common-ui/es/tippy');
  initTippy(app);

  app.use(router);

  const { MotionPlugin } = await import('@vben/plugins/motion');
  app.use(MotionPlugin);

  watchEffect(() => {
    if (preferences.app.dynamicTitle) {
      const routeTitle = router.currentRoute.value.meta?.title;
      const pageTitle =
        (routeTitle ? `${$t(routeTitle)} - ` : '') + preferences.app.name;
      useTitle(pageTitle);
    }
  });

  app.mount('#app');
}

export { bootstrap };
