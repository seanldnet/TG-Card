<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useRouter } from 'vue-router';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';

interface Order { id:string; orderNo:string; productName:string; status:string; createdAt:string; }
interface Ticket { id:string; ticketNo:string; status:string; category:string; }
interface Product { id:string; name:string; status:number; stockCount:number; }
const router=useRouter();
const loading=ref(false),orders=ref<Order[]>([]),tickets=ref<Ticket[]>([]),products=ref<Product[]>([]);
const pendingOrders=computed(()=>orders.value.filter(x=>x.status==='pending_payment'));
const deliveryIssues=computed(()=>orders.value.filter(x=>['paid','delivery_failed'].includes(x.status)));
const openTickets=computed(()=>tickets.value.filter(x=>!['resolved','rejected','closed'].includes(x.status)));
const lowStock=computed(()=>products.value.filter(x=>x.status===1&&x.stockCount===0));
const tasks=computed(()=>[
 {title:'待处理发货',count:deliveryIssues.value.length,desc:'付款完成或发货失败，需要检查并处理',path:'/orders',icon:'mdi:truck-delivery-outline'},
 {title:'待处理售后',count:openTickets.value.length,desc:'用户提交的售后请求，及时回复可减少纠纷',path:'/support',icon:'mdi:headset'},
 {title:'待付款订单',count:pendingOrders.value.length,desc:'正在等待用户完成付款的订单',path:'/orders',icon:'mdi:clock-outline'},
 {title:'缺货商品',count:lowStock.value.length,desc:'已上架但没有可售卡密的商品',path:'/catalog',icon:'mdi:package-variant-closed'},
]);
async function load(){
 loading.value=true;
 const results=await Promise.allSettled([
  requestClient.get<Order[]>('/orders'),
  requestClient.get<Ticket[]>('/support/tickets'),
  requestClient.get<Product[]>('/catalog/products'),
 ]);
 if(results[0].status==='fulfilled')orders.value=results[0].value;
 if(results[1].status==='fulfilled')tickets.value=results[1].value;
 if(results[2].status==='fulfilled')products.value=results[2].value;
 if(results.every(x=>x.status==='rejected'))message.error('运营数据加载失败，请检查登录状态');
 else if(results.some(x=>x.status==='rejected'))message.warning('部分数据暂时无法读取');
 loading.value=false;
}
function open(path:string){void router.push(path)}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">运营工作台</h1><p class="mb-0 mt-1 text-xs text-gray-500">集中查看需要处理的业务事项，点击卡片直接进入对应页面</p></div>
   <a-button size="small" :loading="loading" @click="load"><template #icon><IconifyIcon icon="mdi:refresh" /></template>刷新待办</a-button>
  </div>
  <a-row :gutter="[12,12]">
   <a-col v-for="task in tasks" :key="task.path+task.title" :xs="12" :md="6">
    <a-card :bordered="false" class="h-full cursor-pointer" hoverable :body-style="{padding:'16px 18px'}" @click="open(task.path)">
     <div class="flex items-center justify-between gap-2"><div class="min-w-0"><div class="text-xs text-gray-500">{{task.title}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{task.count}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon :icon="task.icon" class="block text-2xl"/></span></div>
     <div class="mt-3 text-xs leading-5 text-gray-500">{{task.desc}}</div>
     <div class="mt-3 flex items-center gap-1 text-xs font-medium text-primary">前往处理 <IconifyIcon icon="mdi:arrow-right" /></div>
    </a-card>
   </a-col>
  </a-row>
  <a-card :bordered="false" :body-style="{padding:'16px 18px'}">
   <div class="mb-3 text-sm font-semibold">常用操作</div>
   <a-space wrap>
    <a-button size="small" @click="open('/bots')"><template #icon><IconifyIcon icon="mdi:robot-outline"/></template>机器人管理</a-button>
    <a-button size="small" @click="open('/catalog')"><template #icon><IconifyIcon icon="mdi:package-variant-closed"/></template>商品与库存</a-button>
    <a-button size="small" @click="open('/payments/transactions')"><template #icon><IconifyIcon icon="mdi:bank-transfer"/></template>支付流水</a-button>
    <a-button size="small" @click="open('/settings/site')"><template #icon><IconifyIcon icon="mdi:cog-outline"/></template>网站设置</a-button>
   </a-space>
  </a-card>
 </div>
</template>
