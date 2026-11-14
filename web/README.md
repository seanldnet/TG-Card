# TG-Card Web Admin

The admin UI is based on the official [Vue Vben Admin](https://github.com/vbenjs/vue-vben-admin) 5.x monorepo.

- Upstream repository: https://github.com/vbenjs/vue-vben-admin
- Pinned upstream commit: `53b47f8ce99ecea11aa04c9c876a2f18c479838f`
- Selected application: `apps/web-antd` (Vue 3 + TypeScript + Vite + Ant Design Vue)
- Local product customizations live in `web/overrides/`; do not edit the submodule directly.
- Apply local overrides with `scripts/apply-vben-overrides.sh`.
- Build the admin app with `scripts/build-admin.sh`.

Clone the repository with submodules:

```bash
git clone --recurse-submodules https://github.com/seanldnet/TG-Card.git
```

If the repository was already cloned:

```bash
git submodule update --init --recursive
```

The public product introduction is served at `/intro` (the root path redirects there). The administrator login remains at `/auth/login`.

This project keeps the Vben upstream framework intact and stores TG-Card-specific route/page overrides in this repository so upstream updates can be reviewed and reapplied explicitly.
