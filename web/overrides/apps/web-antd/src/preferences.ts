import { defineOverridesPreferences } from '@vben/preferences';

/**
 * TG-Card uses the backend-driven menu API.
 * Keep this setting aligned with src/router/access.ts and GET /api/menu/all.
 */
export const overridesPreferences = defineOverridesPreferences({
  app: {
    accessMode: 'backend',
  },
});
