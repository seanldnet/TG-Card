<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';

import { message, Modal } from 'ant-design-vue';

import { requestClient } from '#/api/request';

interface Bot {
  id: string;
  name: string;
  username: string;
  status: number;
  welcomeText: string;
  createdAt: string;
  updatedAt: string;
}

const loading = ref(false);
const saving = ref(false);
const modalOpen = ref(false);
const editingId = ref('');
const bots = ref<Bot[]>([]);
const form = reactive({ name: '', token: '', welcomeText: '欢迎来到我们的商城！', status: 1 });

async function loadBots() {
  loading.value = true;
  try {
    bots.value = await requestClient.get<Bot[]>('/bots');
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editingId.value = '';
  Object.assign(form, { name: '', token: '', welcomeText: '欢迎来到我们的商城！', status: 1 });
  modalOpen.value = true;
}

function openEdit(bot: Bot) {
  editingId.value = bot.id;
  Object.assign(form, { name: bot.name, token: '', welcomeText: bot.welcomeText, status: bot.status });
  modalOpen.value = true;
}

async function submit() {
  if (!form.name.trim()) {
    message.warning('请输入机器人名称');
    return;
  }
  if (!editingId.value && !form.token.trim()) {
    message.warning('请输入 Telegram Bot Token');
    return;
  }
  saving.value = true;
  try {
    if (editingId.value) {
      await requestClient.put(`/bots/${editingId.value}`, {
        name: form.name,
        welcomeText: form.welcomeText,
        status: form.status,
      });
      message.success('机器人配置已更新');
    } else {
      await requestClient.post('/bots', {
        name: form.name,
        token: form.token,
        welcomeText: form.welcomeText,
      });
      message.success('机器人已添加，Token 已加密保存');
    }
    modalOpen.value = false;
    await loadBots();
  } finally {
    saving.value = false;
  }
}

function removeBot(bot: Bot) {
  Modal.confirm({
    title: '删除机器人',
    content: `确定删除 @${bot.username} 吗？此操作不可撤销。`,
    okType: 'danger',
    onOk: async () => {
      await requestClient.delete(`/bots/${bot.id}`);
      message.success('机器人已删除');
      await loadBots();
    },
  });
}

onMounted(loadBots);
</script>

<template>
  <div class="p-5">
    <a-card :bordered="false">
      <template #title>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-lg font-semibold">Telegram 机器人</div>
            <div class="mt-1 text-sm text-gray-500">
              管理商城销售入口。所有机器人共享商品、库存、订单和用户数据。
            </div>
          </div>
          <a-button type="primary" @click="openCreate">
            <span class="mr-1">+</span> 添加机器人
          </a-button>
        </div>
      </template>

      <a-alert
        class="mb-4"
        type="info"
        show-icon
        message="Token 安全说明"
        description="添加时会实时调用 Telegram getMe 验证 Token，并使用 AES-256-GCM 加密保存。Token 不会在列表中再次显示。"
      />

      <a-table
        :data-source="bots"
        :loading="loading"
        :pagination="{ pageSize: 10, showSizeChanger: true }"
        row-key="id"
        :scroll="{ x: 760 }"
      >
        <a-table-column key="name" title="机器人名称" data-index="name" />
        <a-table-column key="username" title="Telegram 用户名">
          <template #default="{ record }">
            <a :href="`https://t.me/${record.username}`" target="_blank" rel="noreferrer">
              @{{ record.username }}
            </a>
          </template>
        </a-table-column>
        <a-table-column key="status" title="状态" width="110">
          <template #default="{ record }">
            <a-tag :color="record.status === 1 ? 'success' : 'default'">
              {{ record.status === 1 ? '已启用' : '已停用' }}
            </a-tag>
          </template>
        </a-table-column>
        <a-table-column key="createdAt" title="创建时间" data-index="createdAt" />
        <a-table-column key="actions" title="操作" fixed="right" width="150">
          <template #default="{ record }">
            <a-space>
              <a-button type="link" size="small" @click="openEdit(record)">编辑</a-button>
              <a-button type="link" danger size="small" @click="removeBot(record)">删除</a-button>
            </a-space>
          </template>
        </a-table-column>
        <template #emptyText>
          <a-empty description="还没有添加机器人" />
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="modalOpen"
      :title="editingId ? '编辑机器人' : '添加 Telegram 机器人'"
      :confirm-loading="saving"
      ok-text="保存"
      cancel-text="取消"
      @ok="submit"
    >
      <a-form layout="vertical">
        <a-form-item label="机器人名称" required>
          <a-input v-model:value="form.name" placeholder="例如：主商城机器人" :maxlength="100" />
        </a-form-item>
        <a-form-item v-if="!editingId" label="Bot Token" required>
          <a-input-password
            v-model:value="form.token"
            autocomplete="new-password"
            placeholder="从 @BotFather 获取的 Token"
          />
        </a-form-item>
        <a-form-item label="欢迎语">
          <a-textarea
            v-model:value="form.welcomeText"
            :rows="4"
            :maxlength="4000"
            show-count
            placeholder="用户首次打开机器人时显示的欢迎内容"
          />
        </a-form-item>
        <a-form-item v-if="editingId" label="运行开关">
          <a-switch
            :checked="form.status === 1"
            checked-children="启用"
            un-checked-children="停用"
            @change="(checked) => (form.status = checked ? 1 : 0)"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>
