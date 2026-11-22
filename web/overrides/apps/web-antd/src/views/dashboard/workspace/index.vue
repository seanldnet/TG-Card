<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { useRouter } from 'vue-router';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

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
 {title:$t('tg-card.dashboard.taskDelivery'),count:deliveryIssues.value.length,desc:$t('tg-card.dashboard.taskDeliveryDesc'),path:'/orders',icon:'mdi:truck-delivery-outline'},
 {title:$t('tg-card.dashboard.taskSupport'),count:openTickets.value.length,desc:$t('tg-card.dashboard.taskSupportDesc'),path:'/support',icon:'mdi:headset'},
 {title:$t('tg-card.dashboard.taskPayment'),count:pendingOrders.value.length,desc:$t('tg-card.dashboard.taskPaymentDesc'),path:'/orders',icon:'mdi:clock-outline'},
 {title:$t('tg-card.dashboard.taskStock'),count:lowStock.value.length,desc:$t('tg-card.dashboard.taskStockDesc'),path:'/catalog',icon:'mdi:package-variant-closed'},
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
 if(results.every(x=>x.status==='rejected'))message.error($t('tg-card.dashboard.loadFailed'));
 else if(results.some(x=>x.status==='rejected'))message.warning($t('tg-card.dashboard.partialLoad'));
 loading.value=false;
}
function open(path:string){void router.push(path)}
onMounted(load);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">{{$t('tg-card.dashboard.workspaceTitle')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.dashboard.workspaceSubtitle')}}</p></div>
   <a-button size="small" :loading="loading" @click="load"><template #icon><IconifyIcon icon="mdi:refresh" /></template>{{$t('tg-card.dashboard.refreshTasks')}}</a-button>
  </div>
  <a-row :gutter="[12,12]">
   <a-col v-for="task in tasks" :key="task.path+task.title" :xs="12" :md="6">
    <a-card :bordered="false" class="h-full cursor-pointer" hoverable :body-style="{padding:'16px 18px'}" @click="open(task.path)">
     <div class="flex items-center justify-between gap-2"><div class="min-w-0"><div class="text-xs text-gray-500">{{task.title}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{task.count}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon :icon="task.icon" class="block text-2xl"/></span></div>
     <div class="mt-3 text-xs leading-5 text-gray-500">{{task.desc}}</div>
     <div class="mt-3 flex items-center gap-1 text-xs font-medium text-primary">{{$t('tg-card.dashboard.openTask')}} <IconifyIcon icon="mdi:arrow-right" /></div>
    </a-card>
   </a-col>
  </a-row>
  <a-card :bordered="false" :body-style="{padding:'16px 18px'}">
   <div class="mb-3 text-sm font-semibold">{{$t('tg-card.dashboard.commonActions')}}</div>
   <a-space wrap>
    <a-button size="small" @click="open('/bots')"><template #icon><IconifyIcon icon="mdi:robot-outline"/></template>{{$t('tg-card.dashboard.botManagement')}}</a-button>
    <a-button size="small" @click="open('/catalog')"><template #icon><IconifyIcon icon="mdi:package-variant-closed"/></template>{{$t('tg-card.dashboard.catalogStock')}}</a-button>
    <a-button size="small" @click="open('/payments/transactions')"><template #icon><IconifyIcon icon="mdi:bank-transfer"/></template>{{$t('tg-card.dashboard.paymentTransactions')}}</a-button>
    <a-button size="small" @click="open('/settings/site')"><template #icon><IconifyIcon icon="mdi:cog-outline"/></template>{{$t('tg-card.dashboard.websiteSettings')}}</a-button>
   </a-space>
  </a-card>
 </div>
</template>
