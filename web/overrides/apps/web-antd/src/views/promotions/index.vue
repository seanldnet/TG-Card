<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface Promotion {
  id:string; name:string; code:string; discountType:'fixed'|'percent';
  discountValue:number; minimumOrderMinor:number; maximumDiscountMinor:number;
  startsAt:string; endsAt:string; usageLimit:number; perUserLimit:number;
  usageCount:number; status:number; createdAt:string;
}
const loading=ref(false),saving=ref(false),modalOpen=ref(false),editingId=ref('');
const items=ref<Promotion[]>([]);
const form=reactive({
 name:'',code:'',discountType:'fixed' as 'fixed'|'percent',discountValue:5,
 minimumOrder:'0.00',maximumDiscount:'0.00',startsAt:'',endsAt:'',
 usageLimit:0,perUserLimit:1,enabled:true,
});
const columns=[
 {title:$t('tg-card.promotions.columns.name'),dataIndex:'name',key:'name',width:160},
 {title:$t('tg-card.promotions.columns.code'),dataIndex:'code',key:'code',width:170},
 {title:$t('tg-card.promotions.columns.discount'),key:'discount',width:160},
 {title:$t('tg-card.promotions.columns.usage'),key:'usage',width:110},
 {title:$t('tg-card.promotions.columns.period'),key:'period',width:250},
 {title:$t('tg-card.promotions.columns.status'),key:'status',width:90},
 {title:$t('tg-card.promotions.columns.actions'),key:'actions',width:150,fixed:'right' as const},
];
function moneyToMinor(value:string){const n=Number(value);return Number.isFinite(n)?Math.round(n*100):-1}
function minorToMoney(value:number){return (value/100).toFixed(2)}
async function load(){
 loading.value=true;
 try{items.value=await requestClient.get<Promotion[]>('/promotions')}
 catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.promotions.loadFailed'))}
 finally{loading.value=false}
}
function reset(){editingId.value='';form.name='';form.code='';form.discountType='fixed';form.discountValue=5;form.minimumOrder='0.00';form.maximumDiscount='0.00';form.startsAt='';form.endsAt='';form.usageLimit=0;form.perUserLimit=1;form.enabled=true}
function add(){reset();modalOpen.value=true}
function edit(item:Promotion){
 editingId.value=item.id;form.name=item.name;form.code=item.code;form.discountType=item.discountType;
 form.discountValue=item.discountValue;form.minimumOrder=minorToMoney(item.minimumOrderMinor);
 form.maximumDiscount=minorToMoney(item.maximumDiscountMinor);form.startsAt=item.startsAt||'';
 form.endsAt=item.endsAt||'';form.usageLimit=item.usageLimit;form.perUserLimit=item.perUserLimit;form.enabled=item.status===1;
 modalOpen.value=true;
}
async function save(){
 const minimum=moneyToMinor(form.minimumOrder),maximum=moneyToMinor(form.maximumDiscount);
 if(!form.name.trim()||form.code.trim().length<4){message.warning($t('tg-card.promotions.nameAndCode'));return}
 if(!Number.isInteger(form.discountValue)||form.discountValue<=0||(form.discountType==='percent'&&form.discountValue>90)){message.warning($t('tg-card.promotions.discountInvalid'));return}
 if(minimum<0||maximum<0||form.usageLimit<0||form.perUserLimit<1){message.warning($t('tg-card.promotions.limitsInvalid'));return}
 saving.value=true;
 const payload={name:form.name.trim(),code:form.code.trim().toUpperCase(),discountType:form.discountType,
  discountValue:form.discountValue,minimumOrderMinor:minimum,maximumDiscountMinor:maximum,
  startsAt:form.startsAt.trim(),endsAt:form.endsAt.trim(),usageLimit:form.usageLimit,
  perUserLimit:form.perUserLimit,status:form.enabled?1:0};
 try{
  if(editingId.value)await requestClient.put('/promotions/'+editingId.value,payload);
  else await requestClient.post('/promotions',payload);
  message.success(editingId.value?$t('tg-card.promotions.updated'):$t('tg-card.promotions.created'));modalOpen.value=false;await load();
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.promotions.saveFailed'))}
 finally{saving.value=false}
}
function remove(item:Promotion){
 Modal.confirm({title:$t('tg-card.promotions.deleteTitle'),content:$t('tg-card.promotions.deleteConfirm',{code:item.code}),okType:'danger',onOk:async()=>{
  try{await requestClient.delete('/promotions/'+item.id);message.success($t('tg-card.promotions.deleted'));await load()}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.promotions.deleteFailed'))}
 }});
}
function discountLabel(item:Promotion){return item.discountType==='percent'?item.discountValue+'% OFF':minorToMoney(item.discountValue)+' USDT'}
function periodLabel(item:Promotion){return [item.startsAt||$t('tg-card.promotions.immediate'),item.endsAt||$t('tg-card.promotions.longTerm')].join($t('tg-card.promotions.periodTo'))}
onMounted(load);
</script>

<template>
 <div class="min-w-0 p-3 md:p-5">
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title>
    <div class="flex flex-wrap items-center justify-between gap-3">
     <div><div class="text-base font-semibold leading-6">{{$t('tg-card.promotions.title')}}</div><div class="mt-1 max-w-2xl whitespace-normal break-words text-xs leading-5 text-gray-500">{{$t('tg-card.promotions.subtitle')}}</div></div>
     <a-button size="small" type="primary" @click="add">{{$t('tg-card.promotions.create')}}</a-button>
    </div>
   </template>
   <a-table :columns="columns" :data-source="items" :loading="loading" :pagination="{pageSize:10}" row-key="id" :scroll="{x:1050}">
    <template #bodyCell="{column,record}">
     <template v-if="column.key==='discount'"><span>{{ discountLabel(record) }}</span><div class="text-xs text-gray-500">{{$t('tg-card.promotions.minOrder')}} {{ minorToMoney(record.minimumOrderMinor) }} USDT<template v-if="record.maximumDiscountMinor>0"> · {{$t('tg-card.promotions.maxOff')}} {{ minorToMoney(record.maximumDiscountMinor) }}</template></div></template>
     <template v-else-if="column.key==='usage'">{{ record.usageCount }}<span v-if="record.usageLimit>0"> / {{ record.usageLimit }}</span><div class="text-xs text-gray-500">{{$t('tg-card.promotions.perUser',{count:record.perUserLimit})}}</div></template>
     <template v-else-if="column.key==='period'"><span class="text-xs">{{ periodLabel(record) }}</span></template>
     <template v-else-if="column.key==='status'"><a-tag :color="record.status===1?'success':'default'">{{ record.status===1?$t('tg-card.promotions.enabledStatus'):$t('tg-card.promotions.disabledStatus') }}</a-tag></template>
     <template v-else-if="column.key==='actions'"><a-space><a-button type="link" size="small" @click="edit(record)">{{$t('tg-card.promotions.edit')}}</a-button><a-button type="link" danger size="small" @click="remove(record)">{{$t('tg-card.promotions.delete')}}</a-button></a-space></template>
    </template>
   </a-table>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="editingId?$t('tg-card.promotions.editTitle'):$t('tg-card.promotions.newTitle')" :confirm-loading="saving" width="680px" @ok="save">
   <a-form layout="vertical" class="grid grid-cols-1 gap-x-4 md:grid-cols-2">
    <a-form-item :label="$t('tg-card.promotions.name')" required><a-input v-model:value="form.name" maxlength="120" :placeholder="$t('tg-card.promotions.namePlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.codeLabel')" required><a-input v-model:value="form.code" maxlength="40" :placeholder="$t('tg-card.promotions.codePlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.type')"><a-select v-model:value="form.discountType"><a-select-option value="fixed">{{$t('tg-card.promotions.fixed')}}</a-select-option><a-select-option value="percent">{{$t('tg-card.promotions.percent')}}</a-select-option></a-select></a-form-item>
    <a-form-item :label="form.discountType==='percent'?$t('tg-card.promotions.percentLabel'):$t('tg-card.promotions.amountLabel')"><a-input-number v-model:value="form.discountValue" :min="1" :max="form.discountType==='percent'?90:1000000" class="w-full"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.minimum')"><a-input v-model:value="form.minimumOrder" :placeholder="$t('tg-card.promotions.minimum')"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.maximum')"><a-input v-model:value="form.maximumDiscount" placeholder="0.00"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.starts')"><a-input v-model:value="form.startsAt" placeholder="2026-12-01T00:00:00Z"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.ends')"><a-input v-model:value="form.endsAt" placeholder="2026-12-31T23:59:59Z"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.usageLimit')"><a-input-number v-model:value="form.usageLimit" :min="0" class="w-full"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.perUserLimit')"><a-input-number v-model:value="form.perUserLimit" :min="1" :max="100" class="w-full"/></a-form-item>
    <a-form-item :label="$t('tg-card.promotions.statusLabel')"><a-switch v-model:checked="form.enabled" :checked-children="$t('tg-card.promotions.enabledStatus')" :un-checked-children="$t('tg-card.promotions.disabledStatus')"/></a-form-item>
   </a-form>
   <div class="text-xs text-gray-500">{{$t('tg-card.promotions.hint')}}</div>
  </a-modal>
 </div>
</template>
