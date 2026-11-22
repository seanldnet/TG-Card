<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { IconifyIcon } from '@vben/icons';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';

interface Category { id:string; name:string; sortOrder:number; status:number; sourceType:string; upstreamCategoryId:string }
interface Product { id:string; categoryId:string; categoryName:string; name:string; description:string; priceMinor:number; currency:string; deliveryType:string; deliveryConfigJson:string; status:number; stockCount:number; sourceType:string; upstreamProductId:string; upstreamSkuId:string; upstreamPriceSnapshotMinor:number; upstreamCurrency:string; upstreamStockQuantity:number; upstreamStockStatus:string; imagePathsJson:string; bulkDiscountJson:string }
const loading=ref(false),saving=ref(false),categorySaving=ref(false),activeTab=ref('products');
const categoryModalOpen=ref(false),productModalOpen=ref(false),importModalOpen=ref(false);
const editingId=ref(''),importProduct=ref<Product|null>(null);
const categories=ref<Category[]>([]),products=ref<Product[]>([]),upstreamEnabled=ref(false);
const categoryFormRef=ref(),productFormRef=ref(),importFormRef=ref();
const categoryForm=reactive({name:''});
const productForm=reactive({name:'',categoryId:'',price:'0.00',description:'',enabled:true,discountText:'',deliveryType:'card',deliveryText:'',inviteChatId:'',inviteExpiryMinutes:60,telegramFileId:''});
const importForm=reactive({content:''});
const {width}=useWindowSize();
const isMobile=computed(()=>width.value<768);
const enabledProducts=computed(()=>products.value.filter(item=>item.status===1).length);
const totalStock=computed(()=>products.value.reduce((sum,item)=>sum+item.stockCount,0));
const categoryRules={name:[{required:true,whitespace:true,message:$t('tg-card.catalog.validation.categoryName'),trigger:'blur'}]};
const productRules={
 name:[{required:true,whitespace:true,message:$t('tg-card.catalog.validation.productName'),trigger:'blur'}],
 price:[{validator:(_rule:any,value:string)=>Number.isFinite(Number(value))&&Number(value)>=0?Promise.resolve():Promise.reject(new Error($t('tg-card.catalog.validation.price'))),trigger:'blur'}],
};
const importRules={content:[{required:true,whitespace:true,message:$t('tg-card.catalog.validation.importKeys'),trigger:'blur'}]};
async function loadAll(){
 loading.value=true;
 try{
  const [cats,items,upstream]=await Promise.all([requestClient.get<Category[]>('/catalog/categories'),requestClient.get<Product[]>('/catalog/products'),requestClient.get<any>('/upstream/config')]);
  categories.value=cats;products.value=items;upstreamEnabled.value=Boolean(upstream.enabled);
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.catalog.errors.load'))}
 finally{loading.value=false}
}
function handleAddCategory(){categoryForm.name='';categoryModalOpen.value=true}
async function saveCategory(){
 try{await categoryFormRef.value?.validate()}catch{return}
 categorySaving.value=true;
 try{
  await requestClient.post('/catalog/categories',{name:categoryForm.name.trim(),sortOrder:categories.value.length});
  categoryModalOpen.value=false;message.success($t('tg-card.catalog.messages.categoryCreated'));await loadAll();
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.catalog.errors.createCategory'))}
 finally{categorySaving.value=false}
}
function removeCategory(item:Category){
 Modal.confirm({title:$t('tg-card.catalog.deleteCategoryTitle'),content:$t('tg-card.catalog.deleteCategoryConfirm',{name:item.name}),okType:'danger',onOk:async()=>{
  try{await requestClient.delete('/catalog/categories/'+item.id);message.success($t('tg-card.catalog.messages.categoryDeleted'));await loadAll()}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.catalog.errors.deleteCategory'))}
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
  if(!Number.isFinite(price)||price<0){message.warning($t('tg-card.catalog.messages.priceInvalid'));return}
  const tiers=[] as Array<{minQuantity:number;discountPercent:number}>;
  for(const line of productForm.discountText.split(/\r?\n/).map(v=>v.trim()).filter(Boolean)){
   const parts=line.split(/[:：,，]/).map(v=>Number(v.trim()));
   if(parts.length!==2||!Number.isInteger(parts[0])||!Number.isInteger(parts[1])||parts[0]<2||parts[0]>100||parts[1]<1||parts[1]>90){
    message.warning($t('tg-card.catalog.messages.tierFormat'));return;
   }
   tiers.push({minQuantity:parts[0],discountPercent:parts[1]});
  }
  tiers.sort((a,b)=>a.minQuantity-b.minQuantity);
  if(tiers.some((t,i)=>i>0&&t.minQuantity===tiers[i-1].minQuantity)){message.warning($t('tg-card.catalog.messages.duplicateTier'));return}
  saving.value=true;
  try{
   let deliveryConfig={} as Record<string,any>;
  if(productForm.deliveryType==='fixed_text'){if(!productForm.deliveryText.trim()){message.warning($t('tg-card.catalog.messages.fixedTextRequired'));return;}deliveryConfig={text:productForm.deliveryText.trim()};}
  if(productForm.deliveryType==='invite_link'){if(!productForm.inviteChatId.trim()){message.warning($t('tg-card.catalog.messages.chatIdRequired'));return;}deliveryConfig={chatId:productForm.inviteChatId.trim(),expireMinutes:Number(productForm.inviteExpiryMinutes)};}
  if(productForm.deliveryType==='file'||productForm.deliveryType==='file_card'){if(!productForm.telegramFileId.trim()){message.warning($t('tg-card.catalog.messages.fileIdRequired'));return;}deliveryConfig={fileId:productForm.telegramFileId.trim()};}
  const payload={categoryId:productForm.categoryId||'',name:productForm.name.trim(),description:productForm.description,priceMinor:Math.round(price*100),currency:'USDT',deliveryType:productForm.deliveryType,deliveryConfig,status:productForm.enabled?1:0,bulkDiscount:tiers};
  if(editingId.value){await requestClient.put('/catalog/products/'+editingId.value,payload);message.success($t('tg-card.catalog.messages.productUpdated'))}
  else{await requestClient.post('/catalog/products',payload);message.success($t('tg-card.catalog.messages.productCreated'))}
  productModalOpen.value=false;await loadAll();
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.catalog.errors.saveProduct'))}
 finally{saving.value=false}
}
async function openImport(item:Product){importProduct.value=item;importForm.content='';importModalOpen.value=true}
async function submitImport(){
 if(!importProduct.value)return;
 try{await importFormRef.value?.validate()}catch{return}
 saving.value=true;
 try{
  const result=await requestClient.post<{inserted:number;duplicates:number}>('/catalog/products/'+importProduct.value.id+'/keys',{content:importForm.content});
  message.success($t('tg-card.catalog.messages.keysImported',{inserted:result.inserted,duplicates:result.duplicates}));
  importModalOpen.value=false;await loadAll();
 }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.catalog.errors.importKeys'))}
 finally{saving.value=false}
}
function removeProduct(item:Product){
 Modal.confirm({title:$t('tg-card.catalog.deleteProductTitle'),content:$t('tg-card.catalog.deleteProductConfirm',{name:item.name}),okType:'danger',onOk:async()=>{
  try{await requestClient.delete('/catalog/products/'+item.id);message.success($t('tg-card.catalog.messages.productDeleted'));await loadAll()}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.catalog.errors.deleteProduct'))}
 }});
}
function money(item:Product){return (item.priceMinor/100).toFixed(2)+' '+item.currency}
function upstreamPriceLabel(item:Product){return item.sourceType==='upstream'?$t('tg-card.catalog.upstreamPrice',{amount:(item.upstreamPriceSnapshotMinor/100).toFixed(2),currency:item.upstreamCurrency}):''}
function imagePath(item:Product){try{return (JSON.parse(item.imagePathsJson||'[]') as string[])[0]||''}catch{return ''}}
function stockLabel(item:Product){if(item.sourceType==='upstream')return item.upstreamStockQuantity<0?$t('tg-card.catalog.unlimited'):String(item.upstreamStockQuantity);return item.deliveryType==='card'||item.deliveryType==='file_card'?String(item.stockCount):$t('tg-card.catalog.unlimited')}
function deliveryLabel(item:Product){if(item.sourceType==='upstream')return $t('tg-card.catalog.upstream');const key='tg-card.catalog.delivery.'+item.deliveryType;const value=$t(key);return value===key?$t('tg-card.catalog.delivery.card'):value}
function rowProps(record:Product){return {class:upstreamEnabled.value&&record.sourceType==='local'?'opacity-50 grayscale':''};}
onMounted(loadAll);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-5 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div class="min-w-0"><h1 class="m-0 text-xl font-semibold tracking-tight">{{$t('tg-card.catalog.title')}}</h1><p class="mb-0 mt-1 text-xs text-gray-500">{{$t('tg-card.catalog.subtitle')}}</p></div>
  </div>
  <a-row :gutter="[12,12]">
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">{{$t('tg-card.catalog.productTotal')}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{products.length}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.catalog.createdProducts')}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:package-variant-closed" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">{{$t('tg-card.catalog.categories')}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{categories.length}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.catalog.categorySelectable')}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:shape-outline" class="block text-2xl" /></span></div></a-card></a-col>
   <a-col :xs="12" :md="8"><a-card :bordered="false" class="h-full rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}"><div class="flex items-center justify-between gap-2"><div><div class="text-xs text-gray-500">{{$t('tg-card.catalog.sellableKeys')}}</div><div class="mt-2 text-2xl font-semibold leading-none">{{totalStock}}</div><div class="mt-2 text-xs text-gray-400">{{$t('tg-card.catalog.keysWaiting')}}</div></div><span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-blue-100 bg-white text-blue-600"><IconifyIcon icon="mdi:archive-outline" class="block text-2xl" /></span></div></a-card></a-col>
  </a-row>
 
  <a-card :bordered="false" class="rounded-xl shadow-sm" :body-style="{padding:'20px 24px'}">
   <a-alert v-if="upstreamEnabled" class="mb-4" type="info" show-icon message="{{$t('tg-card.catalog.upstreamNotice')}}" />
   <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
    <div class="text-sm font-semibold">{{$t('tg-card.catalog.title')}}</div>
    <a-space wrap>
     <a-button size="middle" @click="handleAddCategory"><template #icon><IconifyIcon icon="mdi:tag-plus-outline" /></template>{{$t('tg-card.catalog.addCategory')}}</a-button>
     <a-button type="primary" size="middle" @click="handleAddProduct"><template #icon><IconifyIcon icon="mdi:package-variant-plus" /></template>{{$t('tg-card.catalog.addProduct')}}</a-button>
    </a-space>
   </div>
   <a-tabs v-model:activeKey="activeTab" class="mb-1">
    <a-tab-pane key="products" :tab="$t('tg-card.catalog.productsTab')"/>
    <a-tab-pane key="categories" :tab="$t('tg-card.catalog.categoryTab')"/>
   </a-tabs>
   <div v-if="activeTab==='categories'">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2"><div><div class="text-sm font-semibold">{{$t('tg-card.catalog.categories')}}</div><div class="mt-1 text-xs text-gray-400">{{$t('tg-card.catalog.categoryHelp')}}</div></div></div>
    <a-space v-if="categories.length" wrap><a-tag v-for="category in categories" :key="category.id" closable @close.prevent="removeCategory(category)">{{category.name}}</a-tag></a-space>
    <a-empty v-else description="{{$t('tg-card.catalog.noCategories')}}"/>
   </div>
   <div v-else>
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2"><div><div class="text-sm font-semibold">{{$t('tg-card.catalog.productList')}}</div><div class="mt-1 text-xs text-gray-400">{{$t('tg-card.catalog.productHelp')}}</div></div></div>
    <div v-if="isMobile" class="flex flex-col gap-3">
     <a-card v-for="item in products" :key="item.id" size="small" class="rounded-lg border border-gray-100" :class="upstreamEnabled&&item.sourceType==='local'?'opacity-50 grayscale':''" :body-style="{padding:'20px 24px'}">
      <div class="flex min-w-0 items-start justify-between gap-3"><div class="flex min-w-0 items-center gap-3"><a-image v-if="imagePath(item)" :src="imagePath(item)" :width="48" :height="48" class="shrink-0 rounded object-cover"/><div class="min-w-0"><div class="break-words text-sm font-semibold">{{item.name}}</div><div class="mt-1 text-xs text-gray-400">{{item.categoryName||'$t('tg-card.catalog.categoryFallback')'}}</div></div></div><div class="flex shrink-0 items-center gap-1"><a-tag class="m-0" :color="item.sourceType==='upstream'?'processing':upstreamEnabled?'default':'blue'">{{item.sourceType==='upstream'?$t('tg-card.catalog.sourceType.upstream'):upstreamEnabled?$t('tg-card.catalog.sourceType.hidden'):$t('tg-card.catalog.sourceType.local')}}</a-tag><a-tag class="m-0" :color="item.status===1?'success':'default'">{{item.status===1?$t('tg-card.catalog.productStatus.active'):$t('tg-card.catalog.productStatus.inactive')}}</a-tag></div></div>
      <a-divider class="my-3"/>
      <div class="grid grid-cols-2 gap-3"><div><div class="text-xs text-gray-400">{{$t('tg-card.catalog.price')}}</div><div class="mt-1 text-sm font-semibold">{{money(item)}}</div><div v-if="upstreamPriceLabel(item)" class="mt-1 text-xs text-gray-400">{{upstreamPriceLabel(item)}}</div></div><div><div class="text-xs text-gray-400">{{$t('tg-card.catalog.stock')}}</div><div class="mt-1 text-sm font-semibold">{{stockLabel(item)}}</div></div><div><div class="text-xs text-gray-400">{{$t('tg-card.catalog.deliveryMethod')}}</div><div class="mt-1 text-sm font-semibold">{{deliveryLabel(item)}}</div></div></div>
      <div class="mt-3 flex flex-wrap justify-end gap-2"><a-button v-if="item.sourceType!=='upstream'&&(item.deliveryType==='card'||item.deliveryType==='file_card')" size="small" @click="openImport(item)">{{$t('tg-card.catalog.import')}}</a-button><a-button size="small" :disabled="item.sourceType==='upstream'" @click="openEdit(item)">{{$t('tg-card.catalog.edit')}}</a-button><a-button size="small" danger :disabled="item.sourceType==='upstream'" @click="removeProduct(item)">{{$t('tg-card.catalog.delete')}}</a-button></div>
     </a-card>
     <a-empty v-if="!loading&&products.length===0" description="{{$t('tg-card.catalog.noProducts')}}"/>
    </div>
    <a-table v-else :data-source="products" :loading="loading" row-key="id" :scroll="{x:1000}" :custom-row="rowProps">
     <a-table-column :title="$t('tg-card.catalog.productName')" data-index="name" key="name" :width="260"><template #default="{record}"><div class="flex items-center gap-2"><a-image v-if="imagePath(record)" :src="imagePath(record)" :width="36" :height="36" class="shrink-0 rounded object-cover"/><span class="whitespace-normal break-words leading-5">{{record.name}}</span></div></template></a-table-column>
     <a-table-column :title="$t('tg-card.catalog.source')" key="source"><template #default="{record}"><a-tag :color="record.sourceType==='upstream'?'processing':upstreamEnabled?'default':'blue'">{{record.sourceType==='upstream'?$t('tg-card.catalog.sourceType.upstream'):upstreamEnabled?$t('tg-card.catalog.sourceType.hidden'):$t('tg-card.catalog.sourceType.local')}}</a-tag></template></a-table-column>
     <a-table-column :title="$t('tg-card.catalog.category')" data-index="categoryName" key="categoryName"><template #default="{record}">{{record.categoryName||'$t('tg-card.catalog.categoryFallback')'}}</template></a-table-column>
     <a-table-column :title="$t('tg-card.catalog.price')" key="price"><template #default="{record}"><div>{{money(record)}}</div><div v-if="upstreamPriceLabel(record)" class="text-xs text-gray-400">{{upstreamPriceLabel(record)}}</div></template></a-table-column>
     <a-table-column :title="$t('tg-card.catalog.stockDelivery')" key="stockCount" :width="140"><template #default="{record}">{{stockLabel(record)}} · {{deliveryLabel(record)}}</template></a-table-column>
     <a-table-column :title="$t('tg-card.catalog.status')" key="status"><template #default="{record}"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?$t('tg-card.catalog.productStatus.active'):$t('tg-card.catalog.productStatus.inactive')}}</a-tag></template></a-table-column>
     <a-table-column :title="$t('tg-card.catalog.actions')" key="actions" fixed="right" width="220"><template #default="{record}"><a-space><a-button v-if="record.sourceType!=='upstream'&&(record.deliveryType==='card'||record.deliveryType==='file_card')" type="link" size="small" @click="openImport(record)">{{$t('tg-card.catalog.import')}}</a-button><a-button type="link" size="small" :disabled="record.sourceType==='upstream'" @click="openEdit(record)">{{$t('tg-card.catalog.edit')}}</a-button><a-button type="link" danger size="small" :disabled="record.sourceType==='upstream'" @click="removeProduct(record)">{{$t('tg-card.catalog.delete')}}</a-button></a-space></template></a-table-column>
     <template #emptyText><a-empty description="{{$t('tg-card.catalog.noProducts')}}"/></template>
    </a-table>
   </div>
  </a-card>
  <a-modal v-model:open="categoryModalOpen" :title="$t('tg-card.catalog.newCategoryTitle')" :width="480" :confirm-loading="categorySaving" :ok-text="$t('tg-card.catalog.saveCategory')" :cancel-text="$t('tg-card.catalog.cancel')" @ok="saveCategory">
   <div class="py-3"><a-form ref="categoryFormRef" :model="categoryForm" :rules="categoryRules" layout="vertical"><a-form-item :label="$t('tg-card.catalog.categoryName')" name="name"><a-input v-model:value="categoryForm.name" size="large" :maxlength="80" :placeholder="$t('tg-card.catalog.categoryPlaceholder')"/></a-form-item></a-form></div>
  </a-modal>
  <a-modal v-model:open="productModalOpen" :title="editingId?$t('tg-card.catalog.editProduct'):$t('tg-card.catalog.newProduct')" :width="560" :confirm-loading="saving" :ok-text="$t('tg-card.catalog.saveProduct')" :cancel-text="$t('tg-card.catalog.cancel')" @ok="saveProduct">
   <div class="py-3"><a-form ref="productFormRef" :model="productForm" :rules="productRules" layout="vertical">
    <a-form-item :label="$t('tg-card.catalog.productName')" name="name"><a-input v-model:value="productForm.name" size="large" :maxlength="160" :placeholder="$t('tg-card.catalog.namePlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.catalog.categories')"><a-select v-model:value="productForm.categoryId" size="large" allow-clear show-search option-filter-prop="label" :placeholder="$t('tg-card.catalog.categoryPlaceholderOptional')"  :options="categories.filter(item=>item.sourceType==='local').map(item=>({label:item.name,value:item.id}))"/></a-form-item>
    <a-form-item :label="$t('tg-card.catalog.priceUSDT')" name="price"><a-input v-model:value="productForm.price" size="large" inputmode="decimal" :placeholder="$t('tg-card.catalog.pricePlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.catalog.deliveryType')"><a-select v-model:value="productForm.deliveryType" size="large" :options="[{label:$t('tg-card.catalog.deliveryOptions.card'),value:'card'},{label:$t('tg-card.catalog.deliveryOptions.fixed_text'),value:'fixed_text'},{label:$t('tg-card.catalog.deliveryOptions.invite_link'),value:'invite_link'},{label:$t('tg-card.catalog.deliveryOptions.file'),value:'file'},{label:$t('tg-card.catalog.deliveryOptions.file_card'),value:'file_card'}]"/></a-form-item>
    <a-form-item v-if="productForm.deliveryType==='fixed_text'" :label="$t('tg-card.catalog.fixedDeliveryText')"><a-textarea v-model:value="productForm.deliveryText" :rows="4" :maxlength="4000" :placeholder="$t('tg-card.catalog.fixedTextPlaceholder')"/></a-form-item>
    <template v-if="productForm.deliveryType==='invite_link'"><a-form-item :label="$t('tg-card.catalog.targetChat')"><a-input v-model:value="productForm.inviteChatId" :placeholder="$t('tg-card.catalog.targetChat')"/></a-form-item><a-form-item :label="$t('tg-card.catalog.inviteExpiry')"><a-input-number v-model:value="productForm.inviteExpiryMinutes" :min="5" :max="10080" class="w-full"/></a-form-item><a-alert type="info" show-icon :message="$t('tg-card.catalog.inviteNotice')"/></template>
    <a-form-item v-if="productForm.deliveryType==='file'||productForm.deliveryType==='file_card'" :label="$t('tg-card.catalog.telegramFileId')"><a-input v-model:value="productForm.telegramFileId" :placeholder="$t('tg-card.catalog.fileIdPlaceholder')"/><div class="mt-1 text-xs text-gray-400">{{$t('tg-card.catalog.fileIdHint')}}</div></a-form-item>
    <a-form-item :label="$t('tg-card.catalog.tierDiscount')"><a-textarea v-model:value="productForm.discountText" :rows="3" :placeholder="$t('tg-card.catalog.tierPlaceholder')"/><div class="mt-1 text-xs text-gray-400">{{$t('tg-card.catalog.tierHint')}}</div></a-form-item>
     <a-form-item :label="$t('tg-card.catalog.description')"><a-textarea v-model:value="productForm.description" :rows="3" :maxlength="4000" show-count :placeholder="$t('tg-card.catalog.descriptionPlaceholder')"/></a-form-item>
    <a-form-item :label="$t('tg-card.catalog.salesStatus')"><a-switch v-model:checked="productForm.enabled" :checked-children="$t('tg-card.catalog.onSale')" :un-checked-children="$t('tg-card.catalog.offSale')"/></a-form-item>
   </a-form></div>
  </a-modal>
  <a-modal v-model:open="importModalOpen" :title="importProduct?$t('tg-card.catalog.importTitle')+' - '+importProduct.name:$t('tg-card.catalog.importTitle')" :width="560" :confirm-loading="saving" :ok-text="$t('tg-card.catalog.startImport')" cancel-text="取消" @ok="submitImport">
   <div class="py-3"><a-alert class="mb-4" type="info" show-icon :message="$t('tg-card.catalog.importNotice')"/><a-form ref="importFormRef" :model="importForm" :rules="importRules" layout="vertical"><a-form-item :label="$t('tg-card.catalog.keyContent')" name="content"><a-textarea v-model:value="importForm.content" :rows="8" :placeholder="$t('tg-card.catalog.keyPlaceholder')"/></a-form-item></a-form></div>
  </a-modal>
 </div>
</template>