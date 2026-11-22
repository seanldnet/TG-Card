<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface RedemptionCode {
 id:string; name:string; code:string; rewardType:'balance'|'package'|'product';
 amountAtomic:number; targetId:string; targetName:string; quantity:number; durationDays:number;
 usageLimit:number; perUserLimit:number; usageCount:number; startsAt:string; endsAt:string; status:number;
}
interface Product { id:string; name:string; priceMinor:number; currency:string; status:number }
const loading=ref(false),saving=ref(false),modalOpen=ref(false),editingId=ref('');
const items=ref<RedemptionCode[]>([]),products=ref<Product[]>([]);
const form=reactive({name:'',code:'',rewardType:'balance' as RedemptionCode['rewardType'],balance:'10.00',targetId:'',targetName:'',quantity:1,durationDays:30,usageLimit:1,perUserLimit:1,startsAt:'',endsAt:'',enabled:true});
const columns=[
 {title:$t('tg-card.redemptionCodes.columns.code'),dataIndex:'code',key:'code',width:180},
 {title:$t('tg-card.redemptionCodes.columns.reward'),key:'reward',width:220},
 {title:$t('tg-card.redemptionCodes.columns.usage'),key:'usage',width:140},
 {title:$t('tg-card.redemptionCodes.columns.period'),key:'period',width:230},
 {title:$t('tg-card.redemptionCodes.columns.status'),key:'status',width:90},
 {title:$t('tg-card.redemptionCodes.columns.actions'),key:'actions',width:130,fixed:'right' as const},
];
const productOptions=computed(()=>products.value.filter(x=>x.status===1).map(x=>({label:x.name+' · '+(x.priceMinor/100).toFixed(2)+' '+x.currency,value:x.id})));
function amountToAtomic(v:string){const n=Number(v);return Number.isFinite(n)?Math.round(n*1_000_000):-1}
function atomicToAmount(v:number){return (v/1_000_000).toFixed(2)}
async function load(){loading.value=true;try{const [codes,goods]=await Promise.all([requestClient.get<RedemptionCode[]>('/redemption-codes'),requestClient.get<Product[]>('/catalog/products')]);items.value=codes;products.value=goods}catch(e:any){message.error(e?.response?.data?.message||$t('tg-card.redemptionCodes.loadFailed'))}finally{loading.value=false}}
function reset(){editingId.value='';form.name='';form.code='';form.rewardType='balance';form.balance='10.00';form.targetId='';form.targetName='';form.quantity=1;form.durationDays=30;form.usageLimit=1;form.perUserLimit=1;form.startsAt='';form.endsAt='';form.enabled=true}
function add(){reset();modalOpen.value=true}
function edit(v:RedemptionCode){editingId.value=v.id;form.name=v.name;form.code=v.code;form.rewardType=v.rewardType;form.balance=atomicToAmount(v.amountAtomic);form.targetId=v.targetId;form.targetName=v.targetName;form.quantity=v.quantity;form.durationDays=v.durationDays;form.usageLimit=v.usageLimit;form.perUserLimit=v.perUserLimit;form.startsAt=v.startsAt;form.endsAt=v.endsAt;form.enabled=v.status===1;modalOpen.value=true}
async function save(){
 if(!form.name.trim()||form.code.trim().length<4){message.warning($t('tg-card.redemptionCodes.nameRequired'));return}
 const amount=amountToAtomic(form.balance);
 if(form.rewardType==='balance'&&(amount<1_000_000||amount>1_000_000_000_000)){message.warning($t('tg-card.redemptionCodes.balanceRange'));return}
 if(form.rewardType==='package'&&!form.targetName.trim()){message.warning($t('tg-card.redemptionCodes.packageName'));return}
 if(form.rewardType==='product'&&!form.targetId){message.warning($t('tg-card.redemptionCodes.productRequired'));return}
 if(form.usageLimit<0||form.perUserLimit<1||form.quantity<1){message.warning($t('tg-card.redemptionCodes.usageInvalid'));return}
 saving.value=true;
 const payload={name:form.name.trim(),code:form.code.trim().toUpperCase(),rewardType:form.rewardType,amountAtomic:form.rewardType==='balance'?amount:0,targetId:form.rewardType==='product'?form.targetId:'',targetName:form.rewardType==='package'?form.targetName.trim():'',quantity:form.quantity,durationDays:form.rewardType==='package'?form.durationDays:0,usageLimit:form.usageLimit,perUserLimit:form.perUserLimit,startsAt:form.startsAt,endsAt:form.endsAt,status:form.enabled?1:0};
 try{if(editingId.value)await requestClient.put('/redemption-codes/'+editingId.value,payload);else await requestClient.post('/redemption-codes',payload);message.success(editingId.value?$t('tg-card.redemptionCodes.updated'):$t('tg-card.redemptionCodes.createdOne'));modalOpen.value=false;await load()}catch(e:any){message.error(e?.response?.data?.message||$t('tg-card.redemptionCodes.saveFailed'))}finally{saving.value=false}
}
function remove(v:RedemptionCode){Modal.confirm({title:$t('tg-card.redemptionCodes.deleteTitle'),content:$t('tg-card.redemptionCodes.deleteConfirm',{code:v.code}),okType:'danger',onOk:async()=>{try{await requestClient.delete('/redemption-codes/'+v.id);message.success($t('tg-card.redemptionCodes.deletedOne'));await load()}catch(e:any){message.error(e?.response?.data?.message||$t('tg-card.redemptionCodes.deleteFailed'))}}})}
function rewardLabel(v:RedemptionCode){if(v.rewardType==='balance')return atomicToAmount(v.amountAtomic)+' '+$t('tg-card.redemptionCodes.balanceReward');if(v.rewardType==='package')return v.targetName+' · '+(v.durationDays>0?$t('tg-card.redemptionCodes.days',{count:v.durationDays}):$t('tg-card.redemptionCodes.permanent'))+' · '+$t('tg-card.redemptionCodes.quantityUnits',{count:v.quantity});const p=products.value.find(x=>x.id===v.targetId);return (p?.name||v.targetName||$t('tg-card.redemptionCodes.productFallback'))+' × '+v.quantity}
function periodLabel(v:RedemptionCode){return [v.startsAt||$t('tg-card.redemptionCodes.immediate'),v.endsAt||$t('tg-card.redemptionCodes.longTerm')].join($t('tg-card.redemptionCodes.periodTo'))}
function copyCode(code:string){navigator.clipboard?.writeText(code).then(()=>message.success($t('tg-card.redemptionCodes.copied'))).catch(()=>message.error($t('tg-card.redemptionCodes.copyFailed')))}
onMounted(load);
</script>
<template>
 <div class="min-w-0 p-3 md:p-5">
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title>
    <div class="flex min-w-0 flex-wrap items-center justify-between gap-3 py-1">
     <div class="min-w-0"><div class="text-base font-semibold leading-6">{{$t('tg-card.redemptionCodes.title')}}</div><div class="mt-1 max-w-2xl whitespace-normal break-words text-xs leading-5 text-gray-500">{{$t('tg-card.redemptionCodes.subtitle')}}</div></div>
     <a-button size="small" type="primary" @click="add">{{$t('tg-card.redemptionCodes.create')}}</a-button>
    </div>
   </template>
   <a-table :columns="columns" :data-source="items" :loading="loading" :pagination="{pageSize:10}" row-key="id" :scroll="{x:950}">
    <template #bodyCell="{column,record}">
     <template v-if="column.key==='reward'">{{rewardLabel(record)}}</template>
     <template v-else-if="column.key==='usage'">{{record.usageCount}}<span v-if="record.usageLimit>0"> / {{record.usageLimit}}</span><div class="text-xs text-gray-500">每人 {{record.perUserLimit}} 次</div></template>
     <template v-else-if="column.key==='period'"><span class="text-xs">{{periodLabel(record)}}</span></template>
     <template v-else-if="column.key==='status'"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?$t('tg-card.redemptionCodes.enabledStatus'):$t('tg-card.redemptionCodes.disabledStatus')}}</a-tag></template>
     <template v-else-if="column.key==='actions'"><a-space size="small"><a-button type="link" size="small" @click="copyCode(record.code)">{{$t('tg-card.redemptionCodes.copy')}}</a-button><a-button type="link" size="small" @click="edit(record)">{{$t('tg-card.redemptionCodes.edit')}}</a-button><a-button type="link" danger size="small" @click="remove(record)">{{$t('tg-card.redemptionCodes.delete')}}</a-button></a-space></template>
    </template>
   </a-table>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="editingId?$t('tg-card.redemptionCodes.editTitle'):$t('tg-card.redemptionCodes.newTitle')" :confirm-loading="saving" width="min(680px, 94vw)" @ok="save">
   <a-form layout="vertical" class="grid grid-cols-1 gap-x-4 md:grid-cols-2">
    <a-form-item :label="$t('tg-card.redemptionCodes.campaignName')" required><a-input v-model:value="form.name" maxlength="120" :placeholder="$t('tg-card.redemptionCodes.campaignPlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.redemptionCodes.code')" required><a-input v-model:value="form.code" maxlength="40" :placeholder="$t('tg-card.redemptionCodes.codePlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.redemptionCodes.rewardType')" class="md:col-span-2"><a-radio-group v-model:value="form.rewardType" button-style="solid"><a-radio-button value="balance">{{$t('tg-card.redemptionCodes.balanceReward')}}</a-radio-button><a-radio-button value="package">{{$t('tg-card.redemptionCodes.packageReward')}}</a-radio-button><a-radio-button value="product">{{$t('tg-card.redemptionCodes.productReward')}}</a-radio-button></a-radio-group></a-form-item>
    <a-form-item v-if="form.rewardType==='balance'" :label="$t('tg-card.redemptionCodes.balanceAmount')" required><a-input v-model:value="form.balance" :placeholder="$t('tg-card.redemptionCodes.balanceAmount')"/></a-form-item>
    <template v-else-if="form.rewardType==='package'">
     <a-form-item :label="$t('tg-card.redemptionCodes.packageNameLabel')" required><a-input v-model:value="form.targetName" :placeholder="$t('tg-card.redemptionCodes.packagePlaceholder')"/></a-form-item>
     <a-form-item :label="$t('tg-card.redemptionCodes.duration')"><a-input-number v-model:value="form.durationDays" :min="0" :max="36500" class="w-full"/></a-form-item>
     <a-form-item :label="$t('tg-card.redemptionCodes.packageQuantity')"><a-input-number v-model:value="form.quantity" :min="1" :max="10000" class="w-full"/></a-form-item>
    </template>
    <template v-else>
     <a-form-item :label="$t('tg-card.redemptionCodes.redeemProduct')" required class="md:col-span-2"><a-select v-model:value="form.targetId" show-search option-filter-prop="label" :options="productOptions" :placeholder="$t('tg-card.redemptionCodes.productPlaceholder')" /></a-form-item>
     <a-form-item :label="$t('tg-card.redemptionCodes.redeemQuantity')"><a-input-number v-model:value="form.quantity" :min="1" :max="100" class="w-full"/></a-form-item>
    </template>
    <a-form-item :label="$t('tg-card.redemptionCodes.usageLimit')"><a-input-number v-model:value="form.usageLimit" :min="0" class="w-full"/></a-form-item>
    <a-form-item :label="$t('tg-card.redemptionCodes.perUserLimit')"><a-input-number v-model:value="form.perUserLimit" :min="1" :max="100" class="w-full"/></a-form-item>
    <a-form-item :label="$t('tg-card.redemptionCodes.startsAt')"><a-input v-model:value="form.startsAt" placeholder="留空立即生效"/></a-form-item>
    <a-form-item :label="$t('tg-card.redemptionCodes.endsAt')"><a-input v-model:value="form.endsAt" placeholder="留空长期有效"/></a-form-item>
    <a-form-item :label="$t('tg-card.redemptionCodes.status')"><a-switch v-model:checked="form.enabled" :checked-children="$t('tg-card.redemptionCodes.enabledStatus')" :un-checked-children="$t('tg-card.redemptionCodes.disabledStatus')"/></a-form-item>
   </a-form>
   <a-alert type="info" show-icon class="mt-2" :message="$t('tg-card.redemptionCodes.explain')"/>
  </a-modal>
 </div>
</template>
