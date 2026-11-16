<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { requestClient } from '#/api/request';

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
 {title:'兑换码',dataIndex:'code',key:'code',width:180},
 {title:'奖励内容',key:'reward',width:220},
 {title:'使用情况',key:'usage',width:140},
 {title:'有效期',key:'period',width:230},
 {title:'状态',key:'status',width:90},
 {title:'操作',key:'actions',width:130,fixed:'right' as const},
];
const productOptions=computed(()=>products.value.filter(x=>x.status===1).map(x=>({label:x.name+' · '+(x.priceMinor/100).toFixed(2)+' '+x.currency,value:x.id})));
function amountToAtomic(v:string){const n=Number(v);return Number.isFinite(n)?Math.round(n*1_000_000):-1}
function atomicToAmount(v:number){return (v/1_000_000).toFixed(2)}
async function load(){loading.value=true;try{const [codes,goods]=await Promise.all([requestClient.get<RedemptionCode[]>('/redemption-codes'),requestClient.get<Product[]>('/catalog/products')]);items.value=codes;products.value=goods}catch(e:any){message.error(e?.response?.data?.message||'兑换码数据加载失败')}finally{loading.value=false}}
function reset(){editingId.value='';form.name='';form.code='';form.rewardType='balance';form.balance='10.00';form.targetId='';form.targetName='';form.quantity=1;form.durationDays=30;form.usageLimit=1;form.perUserLimit=1;form.startsAt='';form.endsAt='';form.enabled=true}
function add(){reset();modalOpen.value=true}
function edit(v:RedemptionCode){editingId.value=v.id;form.name=v.name;form.code=v.code;form.rewardType=v.rewardType;form.balance=atomicToAmount(v.amountAtomic);form.targetId=v.targetId;form.targetName=v.targetName;form.quantity=v.quantity;form.durationDays=v.durationDays;form.usageLimit=v.usageLimit;form.perUserLimit=v.perUserLimit;form.startsAt=v.startsAt;form.endsAt=v.endsAt;form.enabled=v.status===1;modalOpen.value=true}
async function save(){
 if(!form.name.trim()||form.code.trim().length<4){message.warning('请填写名称和至少 4 位兑换码');return}
 const amount=amountToAtomic(form.balance);
 if(form.rewardType==='balance'&&(amount<1_000_000||amount>1_000_000_000_000)){message.warning('USDT 奖励金额须在 1 至 1,000,000 之间');return}
 if(form.rewardType==='package'&&!form.targetName.trim()){message.warning('请填写套餐名称');return}
 if(form.rewardType==='product'&&!form.targetId){message.warning('请选择要兑换的商品');return}
 if(form.usageLimit<0||form.perUserLimit<1||form.quantity<1){message.warning('兑换次数和数量设置无效');return}
 saving.value=true;
 const payload={name:form.name.trim(),code:form.code.trim().toUpperCase(),rewardType:form.rewardType,amountAtomic:form.rewardType==='balance'?amount:0,targetId:form.rewardType==='product'?form.targetId:'',targetName:form.rewardType==='package'?form.targetName.trim():'',quantity:form.quantity,durationDays:form.rewardType==='package'?form.durationDays:0,usageLimit:form.usageLimit,perUserLimit:form.perUserLimit,startsAt:form.startsAt,endsAt:form.endsAt,status:form.enabled?1:0};
 try{if(editingId.value)await requestClient.put('/redemption-codes/'+editingId.value,payload);else await requestClient.post('/redemption-codes',payload);message.success(editingId.value?'兑换码已更新':'兑换码已创建');modalOpen.value=false;await load()}catch(e:any){message.error(e?.response?.data?.message||'保存兑换码失败')}finally{saving.value=false}
}
function remove(v:RedemptionCode){Modal.confirm({title:'删除兑换码',content:'确定删除 '+v.code+' 吗？已完成的兑换记录和用户权益不会被撤销。',okType:'danger',onOk:async()=>{try{await requestClient.delete('/redemption-codes/'+v.id);message.success('兑换码已删除');await load()}catch(e:any){message.error(e?.response?.data?.message||'删除失败')}}})}
function rewardLabel(v:RedemptionCode){if(v.rewardType==='balance')return atomicToAmount(v.amountAtomic)+' USDT 余额';if(v.rewardType==='package')return v.targetName+' · '+(v.durationDays>0?v.durationDays+' 天':'永久')+' · '+v.quantity+' 份';const p=products.value.find(x=>x.id===v.targetId);return (p?.name||v.targetName||'指定商品')+' × '+v.quantity}
function periodLabel(v:RedemptionCode){return [v.startsAt||'立即生效',v.endsAt||'长期有效'].join(' 至 ')}
function copyCode(code:string){navigator.clipboard?.writeText(code).then(()=>message.success('兑换码已复制')).catch(()=>message.error('复制失败'))}
onMounted(load);
</script>
<template>
 <div class="min-w-0 p-3 md:p-5">
  <a-card :bordered="false" :head-style="{ padding: '12px 20px' }" :body-style="{ padding: '16px 20px' }">
   <template #title>
    <div class="flex min-w-0 flex-wrap items-center justify-between gap-3 py-1">
     <div class="min-w-0"><div class="text-base font-semibold leading-6">兑换码管理</div><div class="mt-1 max-w-2xl whitespace-normal break-words text-xs leading-5 text-gray-500">创建可兑换 USDT 余额、套餐权益或指定商品的兑换码，独立于优惠券使用。</div></div>
     <a-button size="small" type="primary" @click="add">新建兑换码</a-button>
    </div>
   </template>
   <a-table :columns="columns" :data-source="items" :loading="loading" :pagination="{pageSize:10}" row-key="id" :scroll="{x:950}">
    <template #bodyCell="{column,record}">
     <template v-if="column.key==='reward'">{{rewardLabel(record)}}</template>
     <template v-else-if="column.key==='usage'">{{record.usageCount}}<span v-if="record.usageLimit>0"> / {{record.usageLimit}}</span><div class="text-xs text-gray-500">每人 {{record.perUserLimit}} 次</div></template>
     <template v-else-if="column.key==='period'"><span class="text-xs">{{periodLabel(record)}}</span></template>
     <template v-else-if="column.key==='status'"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?'启用':'停用'}}</a-tag></template>
     <template v-else-if="column.key==='actions'"><a-space size="small"><a-button type="link" size="small" @click="copyCode(record.code)">复制</a-button><a-button type="link" size="small" @click="edit(record)">编辑</a-button><a-button type="link" danger size="small" @click="remove(record)">删除</a-button></a-space></template>
    </template>
   </a-table>
  </a-card>
  <a-modal v-model:open="modalOpen" :title="editingId?'编辑兑换码':'新建兑换码'" :confirm-loading="saving" width="min(680px, 94vw)" @ok="save">
   <a-form layout="vertical" class="grid grid-cols-1 gap-x-4 md:grid-cols-2">
    <a-form-item label="兑换活动名称" required><a-input v-model:value="form.name" maxlength="120" placeholder="例如：新用户福利"/></a-form-item>
    <a-form-item label="兑换码" required><a-input v-model:value="form.code" maxlength="40" placeholder="例如：GIFT2026"/></a-form-item>
    <a-form-item label="奖励类型" class="md:col-span-2"><a-radio-group v-model:value="form.rewardType" button-style="solid"><a-radio-button value="balance">USDT 余额</a-radio-button><a-radio-button value="package">套餐权益</a-radio-button><a-radio-button value="product">指定商品</a-radio-button></a-radio-group></a-form-item>
    <a-form-item v-if="form.rewardType==='balance'" label="赠送余额（USDT）" required><a-input v-model:value="form.balance" placeholder="10.00"/></a-form-item>
    <template v-else-if="form.rewardType==='package'">
     <a-form-item label="套餐名称" required><a-input v-model:value="form.targetName" placeholder="例如：专业版套餐"/></a-form-item>
     <a-form-item label="有效天数（0 表示永久）"><a-input-number v-model:value="form.durationDays" :min="0" :max="36500" class="w-full"/></a-form-item>
     <a-form-item label="套餐份数"><a-input-number v-model:value="form.quantity" :min="1" :max="10000" class="w-full"/></a-form-item>
    </template>
    <template v-else>
     <a-form-item label="兑换商品" required class="md:col-span-2"><a-select v-model:value="form.targetId" show-search option-filter-prop="label" :options="productOptions" placeholder="选择商品" /></a-form-item>
     <a-form-item label="可兑换数量"><a-input-number v-model:value="form.quantity" :min="1" :max="100" class="w-full"/></a-form-item>
    </template>
    <a-form-item label="总兑换次数（0 表示不限）"><a-input-number v-model:value="form.usageLimit" :min="0" class="w-full"/></a-form-item>
    <a-form-item label="每位用户可兑换次数"><a-input-number v-model:value="form.perUserLimit" :min="1" :max="100" class="w-full"/></a-form-item>
    <a-form-item label="开始时间（RFC3339，可留空）"><a-input v-model:value="form.startsAt" placeholder="留空立即生效"/></a-form-item>
    <a-form-item label="结束时间（RFC3339，可留空）"><a-input v-model:value="form.endsAt" placeholder="留空长期有效"/></a-form-item>
    <a-form-item label="状态"><a-switch v-model:checked="form.enabled" checked-children="启用" un-checked-children="停用"/></a-form-item>
   </a-form>
   <a-alert type="info" show-icon class="mt-2" message="兑换码与优惠券分开管理。兑换记录会保留用户、奖励类型和兑换时间；删除兑换码不会撤销已发放权益。"/>
  </a-modal>
 </div>
</template>
