<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { requestClient } from '#/api/request';

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
 {title:'活动名称',dataIndex:'name',key:'name',width:160},
 {title:'优惠券代码',dataIndex:'code',key:'code',width:170},
 {title:'优惠规则',key:'discount',width:160},
 {title:'使用次数',key:'usage',width:110},
 {title:'有效期',key:'period',width:250},
 {title:'状态',key:'status',width:90},
 {title:'操作',key:'actions',width:150,fixed:'right' as const},
];
function moneyToMinor(value:string){const n=Number(value);return Number.isFinite(n)?Math.round(n*100):-1}
function minorToMoney(value:number){return (value/100).toFixed(2)}
async function load(){
 loading.value=true;
 try{items.value=await requestClient.get<Promotion[]>('/promotions')}
 catch(error:any){message.error(error?.response?.data?.message||'优惠券加载失败')}
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
 if(!form.name.trim()||form.code.trim().length<4){message.warning('请填写活动名称和至少 4 位优惠码');return}
 if(!Number.isInteger(form.discountValue)||form.discountValue<=0||(form.discountType==='percent'&&form.discountValue>90)){message.warning('优惠额度无效；折扣百分比须为 1–90');return}
 if(minimum<0||maximum<0||form.usageLimit<0||form.perUserLimit<1){message.warning('门槛金额或使用限制无效');return}
 saving.value=true;
 const payload={name:form.name.trim(),code:form.code.trim().toUpperCase(),discountType:form.discountType,
  discountValue:form.discountValue,minimumOrderMinor:minimum,maximumDiscountMinor:maximum,
  startsAt:form.startsAt.trim(),endsAt:form.endsAt.trim(),usageLimit:form.usageLimit,
  perUserLimit:form.perUserLimit,status:form.enabled?1:0};
 try{
  if(editingId.value)await requestClient.put('/promotions/'+editingId.value,payload);
  else await requestClient.post('/promotions',payload);
  message.success(editingId.value?'优惠券已更新':'优惠券已创建');modalOpen.value=false;await load();
 }catch(error:any){message.error(error?.response?.data?.message||'保存优惠券失败')}
 finally{saving.value=false}
}
function remove(item:Promotion){
 Modal.confirm({title:'删除优惠券',content:'确定删除 '+item.code+' 吗？已生成订单的优惠快照不会改变。',okType:'danger',onOk:async()=>{
  try{await requestClient.delete('/promotions/'+item.id);message.success('优惠券已删除');await load()}
  catch(error:any){message.error(error?.response?.data?.message||'删除失败')}
 }});
}
function discountLabel(item:Promotion){return item.discountType==='percent'?item.discountValue+'% OFF':minorToMoney(item.discountValue)+' USDT'}
function periodLabel(item:Promotion){return [item.startsAt||'立即生效',item.endsAt||'长期有效'].join(' 至 ')}
onMounted(load);
</script>

<template>
 <div class="min-w-0 p-3 md:p-5">
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title>
    <div class="flex flex-wrap items-center justify-between gap-3">
     <div><div class="text-base font-semibold leading-6">优惠券管理</div><div class="mt-1 max-w-2xl whitespace-normal break-words text-xs leading-5 text-gray-500">设置满减、固定金额立减和折扣优惠，Telegram 用户下单时可使用。</div></div>
     <a-button size="small" type="primary" @click="add">新建优惠券</a-button>
    </div>
   </template>
   <a-table :columns="columns" :data-source="items" :loading="loading" :pagination="{pageSize:10}" row-key="id" :scroll="{x:1050}">
    <template #bodyCell="{column,record}">
     <template v-if="column.key==='discount'"><span>{{ discountLabel(record) }}</span><div class="text-xs text-gray-500">满 {{ minorToMoney(record.minimumOrderMinor) }} USDT<template v-if="record.maximumDiscountMinor>0"> · 最多减 {{ minorToMoney(record.maximumDiscountMinor) }}</template></div></template>
     <template v-else-if="column.key==='usage'">{{ record.usageCount }}<span v-if="record.usageLimit>0"> / {{ record.usageLimit }}</span><div class="text-xs text-gray-500">每人 {{ record.perUserLimit }} 次</div></template>
     <template v-else-if="column.key==='period'"><span class="text-xs">{{ periodLabel(record) }}</span></template>
     <template v-else-if="column.key==='status'"><a-tag :color="record.status===1?'success':'default'">{{ record.status===1?'启用':'停用' }}</a-tag></template>
     <template v-else-if="column.key==='actions'"><a-space><a-button type="link" size="small" @click="edit(record)">编辑</a-button><a-button type="link" danger size="small" @click="remove(record)">删除</a-button></a-space></template>
    </template>
   </a-table>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="editingId?'编辑优惠券':'新建优惠券'" :confirm-loading="saving" width="680px" @ok="save">
   <a-form layout="vertical" class="grid grid-cols-1 gap-x-4 md:grid-cols-2">
    <a-form-item label="活动名称" required><a-input v-model:value="form.name" maxlength="120" placeholder="例如：新用户首单优惠"/></a-form-item>
    <a-form-item label="优惠券代码" required><a-input v-model:value="form.code" maxlength="40" placeholder="例如：WELCOME10"/></a-form-item>
    <a-form-item label="优惠类型"><a-select v-model:value="form.discountType"><a-select-option value="fixed">固定金额立减（USDT）</a-select-option><a-select-option value="percent">订单折扣（百分比）</a-select-option></a-select></a-form-item>
    <a-form-item :label="form.discountType==='percent'?'折扣百分比':'立减金额'"><a-input-number v-model:value="form.discountValue" :min="1" :max="form.discountType==='percent'?90:1000000" class="w-full"/></a-form-item>
    <a-form-item label="最低订单金额（USDT）"><a-input v-model:value="form.minimumOrder" placeholder="0.00"/></a-form-item>
    <a-form-item label="最高优惠金额（USDT，0 表示不限）"><a-input v-model:value="form.maximumDiscount" placeholder="0.00"/></a-form-item>
    <a-form-item label="开始时间（RFC3339，可留空）"><a-input v-model:value="form.startsAt" placeholder="2026-12-01T00:00:00Z"/></a-form-item>
    <a-form-item label="结束时间（RFC3339，可留空）"><a-input v-model:value="form.endsAt" placeholder="2026-12-31T23:59:59Z"/></a-form-item>
    <a-form-item label="总使用次数上限（0 表示不限）"><a-input-number v-model:value="form.usageLimit" :min="0" class="w-full"/></a-form-item>
    <a-form-item label="每位用户使用次数"><a-input-number v-model:value="form.perUserLimit" :min="1" :max="100" class="w-full"/></a-form-item>
    <a-form-item label="活动状态"><a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="停用"/></a-form-item>
   </a-form>
   <div class="text-xs text-gray-500">优惠码在订单创建时锁定名额；订单付款后计为已使用，订单过期则释放名额。为避免零金额订单，优惠金额不能达到订单应付金额。</div>
  </a-modal>
 </div>
</template>
