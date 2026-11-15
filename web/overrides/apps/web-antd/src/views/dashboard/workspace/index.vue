<script setup lang="ts">
import { useRouter } from 'vue-router';
const router = useRouter();
const items = [
  { title:'机器人管理', description:'添加 Telegram Bot、检查运行状态并配置欢迎语', path:'/bots' },
  { title:'商品与库存', description:'管理商品分类、商品价格及卡密库存', path:'/catalog' },
  { title:'订单管理', description:'查看订单、付款状态和发货异常', path:'/orders' },
  { title:'支付交易流水', description:'查看链上到账记录并处理人工核对', path:'/payments/transactions' },
  { title:'售后工单', description:'受理和跟进用户提交的售后请求', path:'/support' },
  { title:'支付设置', description:'在后台配置 TronGrid API Key 和 TRON 收款地址', path:'/settings/payment' },
];
</script>
<template>
  <div class="mx-auto w-full min-w-0 max-w-7xl p-3 md:p-6">
    <h1 class="m-0 text-2xl font-semibold">运营工作台</h1>
    <p class="mt-1 mb-5 text-sm text-gray-500">TG-Card 日常运营功能入口</p>
    <a-row :gutter="[16,16]">
      <a-col v-for="item in items" :key="item.path" :xs="24" :sm="12" :lg="8">
        <a-card hoverable @click="router.push(item.path)">
          <div class="text-lg font-semibold">{{ item.title }}</div>
          <p class="mt-2 mb-0 text-sm text-gray-500">{{ item.description }}</p>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>
