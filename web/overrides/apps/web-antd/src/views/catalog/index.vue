<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';

import { message, Modal } from 'ant-design-vue';

import { requestClient } from '#/api/request';

interface Category {
  id: string;
  name: string;
  sortOrder: number;
  status: number;
}
interface Product {
  id: string;
  categoryId: string;
  categoryName: string;
  name: string;
  description: string;
  priceMinor: number;
  currency: string;
  status: number;
  stockCount: number;
}
const loading = ref(false);
const saving = ref(false);
const modalOpen = ref(false);
const importOpen = ref(false);
const editingId = ref('');
const categories = ref<Category[]>([]);
const products = ref<Product[]>([]);
const importProduct = ref<Product | null>(null);
const keyContent = ref('');
const form = reactive({ categoryId: '', name: '', description: '', price: '0.00', status: 1 });

const enabledProducts = computed(() => products.value.filter((p) => p.status === 1).length);
const totalStock = computed(() => products.value.reduce((sum, p) => sum + p.stockCount, 0));

async function loadAll() {
  loading.value = true;
  try {
    const [categoryData, productData] = await Promise.all([
      requestClient.get<Category[]>('/catalog/categories'),
      requestClient.get<Product[]>('/catalog/products'),
    ]);
    categories.value = categoryData;
    products.value = productData;
  } finally {
    loading.value = false;
  }
}
function openCreate() {
  editingId.value = '';
  Object.assign(form, { categoryId: '', name: '', description: '', price: '0.00', status: 1 });
  modalOpen.value = true;
}
function openEdit(item: Product) {
  editingId.value = item.id;
  Object.assign(form, {
    categoryId: item.categoryId,
    name: item.name,
    description: item.description,
    price: (item.priceMinor / 100).toFixed(2),
    status: item.status,
  });
  modalOpen.value = true;
}
async function saveProduct() {
  const price = Number(form.price);
  if (!form.name.trim() || !Number.isFinite(price) || price < 0) {
    message.warning('请填写商品名称和有效价格');
    return;
  }
  saving.value = true;
  try {
    const payload = {
      categoryId: form.categoryId,
      name: form.name.trim(),
      description: form.description,
      priceMinor: Math.round(price * 100),
      currency: 'USDT',
      deliveryType: 'card',
      status: form.status,
    };
    if (editingId.value) {
      await requestClient.put(`/catalog/products/${editingId.value}`, payload);
      message.success('商品已更新');
    } else {
      await requestClient.post('/catalog/products', payload);
      message.success('商品已创建');
    }
    modalOpen.value = false;
    await loadAll();
  } finally {
    saving.value = false;
  }
}
async function addCategory() {
  let name = '';
  Modal.confirm({
    title: '新增商品分类',
    content: '请在确认框中输入分类名称',
    okText: '继续',
    cancelText: '取消',
    onOk: async () => {
      name = window.prompt('分类名称')?.trim() ?? '';
      if (!name) return;
      await requestClient.post('/catalog/categories', { name, sortOrder: categories.value.length });
      message.success('分类已创建');
      await loadAll();
    },
  });
}
function removeCategory(item: Category) {
  Modal.confirm({
    title: '删除分类',
    content: `确定删除“${item.name}”吗？该分类下商品会自动变为未分类。`,
    okType: 'danger',
    onOk: async () => {
      await requestClient.delete(`/catalog/categories/${item.id}`);
      await loadAll();
      message.success('分类已删除');
    },
  });
}
function openImport(item: Product) {
  importProduct.value = item;
  keyContent.value = '';
  importOpen.value = true;
}
async function submitImport() {
  if (!importProduct.value || !keyContent.value.trim()) {
    message.warning('请粘贴需要导入的卡密，每行一条');
    return;
  }
  saving.value = true;
  try {
    const result = await requestClient.post<{ inserted: number; duplicates: number }>(
      `/catalog/products/${importProduct.value.id}/keys`,
      { content: keyContent.value },
    );
    message.success(`成功新增 ${result.inserted} 条，重复跳过 ${result.duplicates} 条`);
    importOpen.value = false;
    await loadAll();
  } finally {
    saving.value = false;
  }
}
function removeProduct(item: Product) {
  Modal.confirm({
    title: '删除商品',
    content: `确定删除“${item.name}”吗？商品关联库存也会被删除。`,
    okType: 'danger',
    onOk: async () => {
      await requestClient.delete(`/catalog/products/${item.id}`);
      await loadAll();
      message.success('商品已删除');
    },
  });
}
onMounted(loadAll);
</script>

<template>
  <div class="p-5">
    <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-3">
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">商品总数</div>
        <div class="mt-2 text-2xl font-semibold">{{ products.length }}</div>
      </a-card>
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">已上架商品</div>
        <div class="mt-2 text-2xl font-semibold">{{ enabledProducts }}</div>
      </a-card>
      <a-card :bordered="false">
        <div class="text-sm text-gray-500">可售卡密库存</div>
        <div class="mt-2 text-2xl font-semibold">{{ totalStock }}</div>
      </a-card>
    </div>
    <a-card :bordered="false" class="mb-4">
      <template #title>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-lg font-semibold">商品分类</div>
            <div class="mt-1 text-sm text-gray-500">分类供所有 Telegram 机器人共享使用</div>
          </div>
          <a-button @click="addCategory">新增分类</a-button>
        </div>
      </template>
      <a-space wrap>
        <a-tag v-for="category in categories" :key="category.id" closable @close.prevent="removeCategory(category)">
          {{ category.name }}
        </a-tag>
        <a-empty v-if="categories.length === 0" description="尚未创建分类" />
      </a-space>
    </a-card>
    <a-card :bordered="false">
      <template #title>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-lg font-semibold">商品与卡密库存</div>
            <div class="mt-1 text-sm text-gray-500">卡密入库后使用 AES-256-GCM 加密保存；重复卡密会自动跳过</div>
          </div>
          <a-button type="primary" @click="openCreate">新增商品</a-button>
        </div>
      </template>
      <a-table :data-source="products" :loading="loading" row-key="id" :scroll="{ x: 900 }">
        <a-table-column title="商品名称" data-index="name" key="name" />
        <a-table-column title="分类" data-index="categoryName" key="categoryName">
          <template #default="{ record }">{{ record.categoryName || '未分类' }}</template>
        </a-table-column>
        <a-table-column title="售价" key="price">
          <template #default="{ record }">{{ (record.priceMinor / 100).toFixed(2) }} {{ record.currency }}</template>
        </a-table-column>
        <a-table-column title="可售库存" data-index="stockCount" key="stockCount" />
        <a-table-column title="状态" key="status">
          <template #default="{ record }">
            <a-tag :color="record.status === 1 ? 'success' : 'default'">{{ record.status === 1 ? '上架' : '下架' }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="操作" key="actions" fixed="right" width="220">
          <template #default="{ record }">
            <a-space>
              <a-button type="link" size="small" @click="openImport(record)">导入卡密</a-button>
              <a-button type="link" size="small" @click="openEdit(record)">编辑</a-button>
              <a-button type="link" danger size="small" @click="removeProduct(record)">删除</a-button>
            </a-space>
          </template>
        </a-table-column>
        <template #emptyText><a-empty description="还没有商品，先创建一个商品再导入卡密" /></template>
      </a-table>
    </a-card>

    <a-modal v-model:open="modalOpen" :title="editingId ? '编辑商品' : '新增商品'" :confirm-loading="saving" ok-text="保存" cancel-text="取消" @ok="saveProduct">
      <a-form layout="vertical">
        <a-form-item label="商品名称" required><a-input v-model:value="form.name" :maxlength="160" placeholder="例如：会员月卡" /></a-form-item>
        <a-form-item label="商品分类"><a-select v-model:value="form.categoryId" allow-clear placeholder="选择分类"><a-select-option v-for="category in categories" :key="category.id" :value="category.id">{{ category.name }}</a-select-option></a-select></a-form-item>
        <a-form-item label="售价（USDT）" required><a-input v-model:value="form.price" placeholder="例如 9.90" /></a-form-item>
        <a-form-item label="商品说明"><a-textarea v-model:value="form.description" :rows="3" :maxlength="4000" /></a-form-item>
        <a-form-item label="上架状态"><a-switch :checked="form.status === 1" checked-children="上架" un-checked-children="下架" @change="(checked) => (form.status = checked ? 1 : 0)" /></a-form-item>
      </a-form>
    </a-modal>
    <a-modal v-model:open="importOpen" :title="`导入卡密 - ${importProduct?.name ?? ''}`" :confirm-loading="saving" ok-text="开始导入" cancel-text="取消" @ok="submitImport">
      <a-alert class="mb-3" type="info" show-icon message="每行一条卡密" description="系统会在入库前加密卡密，并按商品维度去重。单批最多 1 MB。" />
      <a-textarea v-model:value="keyContent" :rows="10" placeholder="KEY-XXXX-XXXX&#10;KEY-YYYY-YYYY&#10;每行粘贴一条卡密" />
    </a-modal>
  </div>
</template>
