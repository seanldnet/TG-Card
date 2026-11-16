<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';

interface Category { id:string; name:string; sortOrder:number; status:number }
interface Product { id:string; categoryId:string; categoryName:string; name:string; description:string; priceMinor:number; currency:string; deliveryType:string; deliveryConfigJson:string; status:number; stockCount:number; bulkDiscountJson:string }
const loading=ref(false),saving=ref(false),categorySaving=ref(false),activeTab=ref('products');
const categoryModalOpen=ref(false),productModalOpen=ref(false),importModalOpen=ref(false);
const editingId=ref(''),importProduct=ref<Product|null>(null);
const categories=ref<Category[]>([]),products=ref<Product[]>([]);
const categoryFormRef=ref(),productFormRef=ref(),importFormRef=ref();
const categoryForm=reactive({name:''});
const productForm=reactive({name:'',categoryId:'',price:'0.00',description:'',enabled:true,discountText:'',deliveryType:'card',deliveryText:'',inviteChatId:'',inviteExpiryMinutes:60,telegramFileId:''});
const importForm=reactive({content:''});
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const enabledProducts=computed(()=>products.value.filter(item=>item.status===1).length);
const totalStock=computed(()=>products.value.reduce((sum,item)=>sum+item.stockCount,0));
const categoryRules={name:[{required:true,whitespace:true,message:'请输入分类名称',trigger:'blur'}]};
const productRules={
 name:[{required:true,whitespace:true,message:'请输入商品名称',trigger:'blur'}],
 price:[{validator:(_rule:any,value:string)=>Number.isFinite(Number(value))&&Number(value)>=0?Promise.resolve():Promise.reject(new Error('请输入有效的商品价格')),trigger:'blur'}],
};
const importRules={content:[{required:true,whitespace:true,message:'请粘贴卡密内容，每行一条',trigger:'blur'}]};
async function loadAll(){
 loading.value=true;
 try{
  const [cats,items]=await Promise.all([requestClient.get<Category[]>('/catalog/categories'),requestClient.get<Product[]>('/catalog/products')]);
  categories.value=cats;products.value=items;
 }catch(error:any){message.error(error?.response?.data?.message||'商品与分类加载失败')}
 finally{loading.value=false}
}
function handleAddCategory(){categoryForm.name='';categoryModalOpen.value=true}
async function saveCategory(){
 try{await categoryFormRef.value?.validate()}catch{return}
 categorySaving.value=true;
 try{
  await requestClient.post('/catalog/categories',{name:categoryForm.name.trim(),sortOrder:categories.value.length});
  categoryModalOpen.value=false;message.success('分类创建成功');await loadAll();
 }catch(error:any){message.error(error?.response?.data?.message||'创建分类失败')}
 finally{categorySaving.value=false}
}
function removeCategory(item:Category){
 Modal.confirm({title:'删除分类',content:'确定删除“'+item.name+'”吗？该分类下的商品会变为未分类。',okType:'danger',onOk:async()=>{
  try{await requestClient.delete('/catalog/categories/'+item.id);message.success('分类已删除');await loadAll()}
  catch(error:any){message.error(error?.response?.data?.message||'删除分类失败')}
 }});
}
function handleAddProduct(){
 editingId.value='';
 productForm.name='';productForm.categoryId='';productForm.price='0.00';productForm.description='';productForm.enabled=true;productForm.discountText='';productForm.deliveryType='card';productForm.deliveryText='';productForm.inviteChatId='';productForm.inviteExpiryMinutes=60;productForm.telegramFileId='';
 productModalOpen.value=true;
}
function openEdit(item:Product){
  editingId.value=item.id;productForm.name=item.name;productForm.categoryId=item.categoryId||'';
  productForm.price=(item.priceMinor/100).toFixed(2);productForm.description=item.description||'';
  productForm.enabled=item.status===1;
  productForm.deliveryType=item.deliveryType||'card';
  productForm.deliveryText='';productForm.inviteChatId='';productForm.inviteExpiryMinutes=60;productForm.telegramFileId='';
  try { const config=JSON.parse(item.deliveryConfigJson||'{}');productForm.deliveryText=config.text||'';productForm.inviteChatId=config.chatId||'';productForm.inviteExpiryMinutes=config.expireMinutes||60;productForm.telegramFileId=config.fileId||''; } catch {}
  try {
   productForm.discountText=(JSON.parse(item.bulkDiscountJson||'[]') as Array<{minQuantity:number;discountPercent:number}>).map(t=>t.minQuantity+':'+t.discountPercent).join('\n');
  } catch { productForm.discountText=''; }
  productModalOpen.value=true;
}
async function saveProduct(){
  try{await productFormRef.value?.validate()}catch{return}
  const price=Number(productForm.price);
  if(!Number.isFinite(price)||price<0){message.warning('请输入有效的商品价格');return}
  const tiers=[] as Array<{minQuantity:number;discountPercent:number}>;
  for(const line of productForm.discountText.split(/\r?\n/).map(v=>v.trim()).filter(Boolean)){
   const parts=line.split(/[:：,，]/).map(v=>Number(v.trim()));
   if(parts.length!==2||!Number.isInteger(parts[0])||!Number.isInteger(parts[1])||parts[0]<2||parts[0]>100||parts[1]<1||parts[1]>90){
    message.warning('阶梯折扣请按“购买数量:折扣百分比”填写，例如 2:10');return;
   }
   tiers.push({minQuantity:parts[0],discountPercent:parts[1]});
  }
  tiers.sort((a,b)=>a.minQuantity-b.minQuantity);
  if(tiers.some((t,i)=>i>0&&t.minQuantity===tiers[i-1].minQuantity)){message.warning('同一购买数量不能重复配置折扣');return}
  saving.value=true;
  try{
   let deliveryConfig={} as Record<string,any>;
  if(productForm.deliveryType==='fixed_text'){if(!productForm.deliveryText.trim()){message.warning('请填写固定文本交付内容');return;}deliveryConfig={text:productForm.deliveryText.trim()};}
  if(productForm.deliveryType==='invite_link'){if(!productForm.inviteChatId.trim()){message.warning('请填写目标群组 Chat ID');return;}deliveryConfig={chatId:productForm.inviteChatId.trim(),expireMinutes:Number(productForm.inviteExpiryMinutes)};}
  if(productForm.deliveryType==='file'||productForm.deliveryType==='file_card'){if(!productForm.telegramFileId.trim()){message.warning('请填写 Telegram 文件 file_id');return;}deliveryConfig={fileId:productForm.telegramFileId.trim()};}
  const payload={categoryId:productForm.categoryId||'',name:productForm.name.trim(),description:productForm.description,priceMinor:Math.round(price*100),currency:'USDT',deliveryType:productForm.deliveryType,deliveryConfig,status:productForm.enabled?1:0,bulkDiscount:tiers};
  if(editingId.value){await requestClient.put('/catalog/products/'+editingId.value,payload);message.success('商品修改成功')}
  else{await requestClient.post('/catalog/products',payload);message.success('商品创建成功')}
  productModalOpen.value=false;await loadAll();
 }catch(error:any){message.error(error?.response?.data?.message||'保存商品失败，请检查分类和填写内容')}
 finally{saving.value=false}
}
async function openImport(item:Product){importProduct.value=item;importForm.content='';importModalOpen.value=true}
async function submitImport(){
 if(!importProduct.value)return;
 try{await importFormRef.value?.validate()}catch{return}
 saving.value=true;
 try{
  const result=await requestClient.post<{inserted:number;duplicates:number}>('/catalog/products/'+importProduct.value.id+'/keys',{content:importForm.content});
  message.success('成功新增 '+result.inserted+' 条卡密，重复跳过 '+result.duplicates+' 条');
  importModalOpen.value=false;await loadAll();
 }catch(error:any){message.error(error?.response?.data?.message||'导入卡密失败')}
 finally{saving.value=false}
}
function removeProduct(item:Product){
 Modal.confirm({title:'删除商品',content:'确定删除“'+item.name+'”吗？商品关联库存也会被删除。',okType:'danger',onOk:async()=>{
  try{await requestClient.delete('/catalog/products/'+item.id);message.success('商品已删除');await loadAll()}
  catch(error:any){message.error(error?.response?.data?.message||'删除商品失败')}
 }});
}
function money(item:Product){return (item.priceMinor/100).toFixed(2)+' '+item.currency}
onMounted(loadAll);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-5 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div class="min-w-0"><h1 class="m-0 text-xl font-semibold tracking-tight">商品与库存</h1><p class="mb-0 mt-1 text-xs text-gray-500">管理商城商品、分类及自动发货卡密库存</p></div>
  </div>
  <a-row :gutter="[12,12]">
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">商品总数</div><div class="mt-2 text-2xl font-semibold leading-none">{{products.length}}</div><div class="mt-2 text-xs text-gray-400">已创建的商品</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:package-variant-closed" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">商品分类</div><div class="mt-2 text-2xl font-semibold leading-none">{{categories.length}}</div><div class="mt-2 text-xs text-gray-400">可供商品选择</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:shape-outline" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">可售卡密库存</div><div class="mt-2 text-2xl font-semibold leading-none">{{totalStock}}</div><div class="mt-2 text-xs text-gray-400">等待自动发货的卡密</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:archive-outline" class="block text-2xl" /></span></div></a-card></a-col>
  </a-row>
 
  <a-card :bordered="false" class="rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}">
   <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
    <div class="text-sm font-semibold">商品与库存</div>
    <a-space wrap>
     <a-button size="middle" @click="handleAddCategory"><template #icon><IconifyIcon icon="mdi:tag-plus-outline" /></template>新增分类</a-button>
     <a-button type="primary" size="middle" @click="handleAddProduct"><template #icon><IconifyIcon icon="mdi:package-variant-plus" /></template>新增商品</a-button>
    </a-space>
   </div>
   <a-tabs v-model:activeKey="activeTab" class="mb-1">
    <a-tab-pane key="products" tab="商品与库存"/>
    <a-tab-pane key="categories" tab="商品分类"/>
   </a-tabs>
   <div v-if="activeTab==='categories'">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2"><div><div class="text-sm font-semibold">商品分类</div><div class="mt-1 text-xs text-gray-400">分类用于整理商品；没有分类也可以直接创建商品</div></div></div>
    <a-space v-if="categories.length" wrap><a-tag v-for="category in categories" :key="category.id" closable @close.prevent="removeCategory(category)">{{category.name}}</a-tag></a-space>
    <a-empty v-else description="还没有分类，点击“新增分类”创建第一个分类"/>
   </div>
   <div v-else>
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2"><div><div class="text-sm font-semibold">商品列表</div><div class="mt-1 text-xs text-gray-400">没有分类也可以新增商品，分类为可选项</div></div></div>
    <div v-if="isMobile" class="flex flex-col gap-3">
     <a-card v-for="item in products" :key="item.id" size="small" class="rounded-lg border border-gray-100" :body-style="{padding:'20px 24px'}">
      <div class="flex min-w-0 items-start justify-between gap-3"><div class="min-w-0"><div class="break-words text-sm font-semibold">{{item.name}}</div><div class="mt-1 text-xs text-gray-400">{{item.categoryName||'未分类'}}</div></div><a-tag class="m-0 shrink-0" :color="item.status===1?'success':'default'">{{item.status===1?'销售中':'已下架'}}</a-tag></div>
      <a-divider class="my-3"/>
      <div class="grid grid-cols-2 gap-3"><div><div class="text-xs text-gray-400">售价</div><div class="mt-1 text-sm font-semibold">{{money(item)}}</div></div><div><div class="text-xs text-gray-400">可售库存</div><div class="mt-1 text-sm font-semibold">{{item.stockCount}}</div></div></div>
      <div class="mt-3 flex flex-wrap justify-end gap-2"><a-button size="small" @click="openImport(item)">导入卡密</a-button><a-button size="small" @click="openEdit(item)">编辑</a-button><a-button size="small" danger @click="removeProduct(item)">删除</a-button></div>
     </a-card>
     <a-empty v-if="!loading&&products.length===0" description="还没有商品，点击“新增商品”创建第一个商品"/>
    </div>
    <a-table v-else :data-source="products" :loading="loading" row-key="id" :scroll="{x:900}">
     <a-table-column title="商品名称" data-index="name" key="name" :width="260"><template #default="{record}"><div class="whitespace-normal break-words leading-5">{{record.name}}</div></template></a-table-column>
     <a-table-column title="分类" data-index="categoryName" key="categoryName"><template #default="{record}">{{record.categoryName||'未分类'}}</template></a-table-column>
     <a-table-column title="售价" key="price"><template #default="{record}">{{money(record)}}</template></a-table-column>
     <a-table-column title="可售库存" data-index="stockCount" key="stockCount" :width="110"/>
     <a-table-column title="状态" key="status"><template #default="{record}"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?'销售中':'已下架'}}</a-tag></template></a-table-column>
     <a-table-column title="操作" key="actions" fixed="right" width="220"><template #default="{record}"><a-space><a-button type="link" size="small" @click="openImport(record)">导入卡密</a-button><a-button type="link" size="small" @click="openEdit(record)">编辑</a-button><a-button type="link" danger size="small" @click="removeProduct(record)">删除</a-button></a-space></template></a-table-column>
     <template #emptyText><a-empty description="还没有商品，点击“新增商品”创建第一个商品"/></template>
    </a-table>
   </div>
  </a-card>
  <a-modal v-model:open="categoryModalOpen" title="新增商品分类" :width="480" :confirm-loading="categorySaving" ok-text="保存分类" cancel-text="取消" @ok="saveCategory">
   <div class="py-3"><a-form ref="categoryFormRef" :model="categoryForm" :rules="categoryRules" layout="vertical"><a-form-item label="分类名称" name="name"><a-input v-model:value="categoryForm.name" size="large" :maxlength="80" placeholder="例如：会员卡、软件授权"/></a-form-item></a-form></div>
  </a-modal>
  <a-modal v-model:open="productModalOpen" :title="editingId?'编辑商品':'新增商品'" :width="560" :confirm-loading="saving" ok-text="保存商品" cancel-text="取消" @ok="saveProduct">
   <div class="py-3"><a-form ref="productFormRef" :model="productForm" :rules="productRules" layout="vertical">
    <a-form-item label="商品名称" name="name"><a-input v-model:value="productForm.name" size="large" :maxlength="160" placeholder="例如：会员月卡"/></a-form-item>
    <a-form-item label="商品分类"><a-select v-model:value="productForm.categoryId" size="large" allow-clear show-search option-filter-prop="label" placeholder="可不选择分类" :options="categories.map(item=>({label:item.name,value:item.id}))"/></a-form-item>
    <a-form-item label="售价（USDT）" name="price"><a-input v-model:value="productForm.price" size="large" inputmode="decimal" placeholder="例如：9.90"/></a-form-item>
    <a-form-item label="商品交付方式"><a-select v-model:value="productForm.deliveryType" size="large" :options="[{label:'卡密自动发货',value:'card'},{label:'固定文本',value:'fixed_text'},{label:'一次性 Telegram 邀请链接',value:'invite_link'},{label:'Telegram 文件',value:'file'},{label:'文件 + 卡密',value:'file_card'}]"/></a-form-item>
    <a-form-item v-if="productForm.deliveryType==='fixed_text'" label="固定交付文本"><a-textarea v-model:value="productForm.deliveryText" :rows="4" :maxlength="4000" placeholder="付款后自动发送给买家的内容"/></a-form-item>
    <template v-if="productForm.deliveryType==='invite_link'"><a-form-item label="目标群组 Chat ID"><a-input v-model:value="productForm.inviteChatId" placeholder="-100xxxxxxxxxx"/></a-form-item><a-form-item label="邀请链接有效期（分钟）"><a-input-number v-model:value="productForm.inviteExpiryMinutes" :min="5" :max="10080" class="w-full"/></a-form-item><a-alert type="info" show-icon message="Bot 必须是目标群组管理员，并拥有邀请用户权限；每个订单生成仅限 1 人使用的专属链接。"/></template>
    <a-form-item v-if="productForm.deliveryType==='file'||productForm.deliveryType==='file_card'" label="Telegram 文件 file_id"><a-input v-model:value="productForm.telegramFileId" placeholder="填写由该 Bot 获取的 Telegram file_id"/><div class="mt-1 text-xs text-gray-400">当前通过 Telegram file_id 发送文件；请先将文件发送给该 Bot 并取得 file_id。</div></a-form-item>
    <a-form-item label="批量购买阶梯折扣（选填）"><a-textarea v-model:value="productForm.discountText" :rows="3" placeholder="每行一档：购买数量:折扣百分比\n例如：2:10 表示购买 2 件及以上打 9 折；5:20 表示购买 5 件及以上打 8 折"/><div class="mt-1 text-xs text-gray-400">折扣比例表示优惠百分比；不填写则按原价结算。数量需递增，最多 20 档。</div></a-form-item>
     <a-form-item label="商品说明"><a-textarea v-model:value="productForm.description" :rows="3" :maxlength="4000" show-count placeholder="填写商品说明（选填）"/></a-form-item>
    <a-form-item label="销售状态"><a-switch v-model:checked="productForm.enabled" checked-children="上架" un-checked-children="下架"/></a-form-item>
   </a-form></div>
  </a-modal>
  <a-modal v-model:open="importModalOpen" :title="importProduct?'导入卡密 - '+importProduct.name:'导入卡密'" :width="560" :confirm-loading="saving" ok-text="开始导入" cancel-text="取消" @ok="submitImport">
   <div class="py-3"><a-alert class="mb-4" type="info" show-icon message="每行填写一条卡密，系统会加密保存并自动跳过重复内容"/><a-form ref="importFormRef" :model="importForm" :rules="importRules" layout="vertical"><a-form-item label="卡密内容" name="content"><a-textarea v-model:value="importForm.content" :rows="8" placeholder="每行一条卡密内容"/></a-form-item></a-form></div>
  </a-modal>
 </div>
</template>