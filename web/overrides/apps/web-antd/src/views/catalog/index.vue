<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { message, Modal } from 'ant-design-vue';
import { useWindowSize } from '@vueuse/core';
import { useVbenForm } from '#/adapter/form';
import { requestClient } from '#/api/request';
interface Category{id:string;name:string;sortOrder:number;status:number}
interface Product{id:string;categoryId:string;categoryName:string;name:string;description:string;priceMinor:number;currency:string;status:number;stockCount:number}
const loading=ref(false),saving=ref(false),categorySaving=ref(false),editingId=ref('');
const categoryModalOpen=ref(false),productModalOpen=ref(false),importModalOpen=ref(false),activeTab=ref('products');
const categories=ref<Category[]>([]),products=ref<Product[]>([]),importProduct=ref<Product|null>(null);
const {width}=useWindowSize(),isMobile=computed(()=>width.value<768);
const enabledProducts=computed(()=>products.value.filter(p=>p.status===1).length);
const totalStock=computed(()=>products.value.reduce((sum,p)=>sum+p.stockCount,0));
const [CategoryForm,categoryFormApi]=useVbenForm({commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',schema:[{component:'Input',fieldName:'name',label:'分类名称',componentProps:{maxlength:80,placeholder:'例如：会员卡、软件授权'}}]});
const [ProductForm,productFormApi]=useVbenForm({
 commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',
 schema:[
  {component:'Input',fieldName:'name',label:'商品名称',componentProps:{maxlength:160,placeholder:'例如：会员月卡'}},
  {component:'Select',fieldName:'categoryId',label:'商品分类',componentProps:{allowClear:true,placeholder:'选择分类',options:computed(()=>categories.value.map(c=>({label:c.name,value:c.id})))}},
  {component:'Input',fieldName:'price',label:'售价（USDT）',componentProps:{placeholder:'例如 9.90'}},
  {component:'Textarea',fieldName:'description',label:'商品说明',componentProps:{rows:3,maxlength:4000}},
  {component:'Switch',fieldName:'enabled',label:'上架状态'},
 ],
});
const [ImportForm,importFormApi]=useVbenForm({commonConfig:{componentProps:{class:'w-full'}},layout:'vertical',showDefaultActions:false,wrapperClass:'grid-cols-1',schema:[{component:'Textarea',fieldName:'content',label:'卡密内容（每行一条）',componentProps:{rows:10,placeholder:'KEY-XXXX-XXXX\nKEY-YYYY-YYYY'}}]});
async function loadAll(){loading.value=true;try{const [cats,items]=await Promise.all([requestClient.get<Category[]>('/catalog/categories'),requestClient.get<Product[]>('/catalog/products')]);categories.value=cats;products.value=items}catch{message.error('商品与分类加载失败')}finally{loading.value=false}}
async function handleAddCategory(){await categoryFormApi.setValues({name:''});categoryModalOpen.value=true}
async function saveCategory(){const {valid}=await categoryFormApi.validate();if(!valid)return;const values=await categoryFormApi.getValues();const name=String(values.name||'').trim();if(!name){message.warning('请输入分类名称');return}categorySaving.value=true;try{await requestClient.post('/catalog/categories',{name,sortOrder:categories.value.length});categoryModalOpen.value=false;message.success('分类已创建');await loadAll()}catch(error:any){message.error(error?.response?.data?.message||'创建分类失败')}finally{categorySaving.value=false}}
function removeCategory(item:Category){Modal.confirm({title:'删除分类',content:`确定删除“${item.name}”吗？该分类下商品会自动变为未分类。`,okType:'danger',onOk:async()=>{await requestClient.delete('/catalog/categories/'+item.id);await loadAll();message.success('分类已删除')}})}
async function handleAddProduct(){editingId.value='';await productFormApi.setValues({name:'',categoryId:undefined,price:'0.00',description:'',enabled:true});productModalOpen.value=true}
async function openEdit(item:Product){editingId.value=item.id;await productFormApi.setValues({name:item.name,categoryId:item.categoryId,price:(item.priceMinor/100).toFixed(2),description:item.description,enabled:item.status===1});productModalApi.open()}
async function saveProduct(){const {valid}=await productFormApi.validate();if(!valid)return;const values=await productFormApi.getValues();const price=Number(values.price);if(!String(values.name||'').trim()||!Number.isFinite(price)||price<0){message.warning('请填写商品名称和有效价格');return}saving.value=true;try{const payload={categoryId:values.categoryId||'',name:String(values.name).trim(),description:values.description||'',priceMinor:Math.round(price*100),currency:'USDT',deliveryType:'card',status:values.enabled?1:0};if(editingId.value){await requestClient.put('/catalog/products/'+editingId.value,payload);message.success('商品已更新')}else{await requestClient.post('/catalog/products',payload);message.success('商品已创建')}productModalOpen.value=false;await loadAll()}catch(error:any){message.error(error?.response?.data?.message||'保存商品失败')}finally{saving.value=false}}
async function openImport(item:Product){importProduct.value=item;await importFormApi.setValues({content:''});importModalOpen.value=true}
async function submitImport(){if(!importProduct.value)return;const {valid}=await importFormApi.validate();if(!valid)return;const values=await importFormApi.getValues();if(!String(values.content||'').trim()){message.warning('请粘贴需要导入的卡密，每行一条');return}saving.value=true;try{const result=await requestClient.post<{inserted:number;duplicates:number}>('/catalog/products/'+importProduct.value.id+'/keys',{content:values.content});message.success(`成功新增 ${result.inserted} 条，重复跳过 ${result.duplicates} 条`);importModalOpen.value=false;await loadAll()}catch(error:any){message.error(error?.response?.data?.message||'导入卡密失败')}finally{saving.value=false}}
function removeProduct(item:Product){Modal.confirm({title:'删除商品',content:`确定删除“${item.name}”吗？商品关联库存也会被删除。`,okType:'danger',onOk:async()=>{await requestClient.delete('/catalog/products/'+item.id);await loadAll();message.success('商品已删除')}})}
function money(item:Product){return (item.priceMinor/100).toFixed(2)+' '+item.currency}
onMounted(loadAll);
</script>
<template>
 <div class="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-4 p-3 md:p-6">
  <a-alert v-if="totalStock===0" class="mb-4" type="warning" show-icon message="当前没有可售卡密库存" description="请在商品列表中导入卡密，库存充足后机器人才能正常自动发货"/>
  <a-row :gutter="[16,16]">
   <a-col :xs="12" :md="8"><a-card :bordered="false" :body-style="{ padding: '20px 24px' }"><div class="text-sm text-gray-500">商品总数</div><div class="mt-3 text-2xl font-bold">{{products.length}}</div><div class="mt-2 text-xs text-gray-500">当前创建的商品</div></a-card></a-col><a-col :xs="12" :md="8"><a-card :bordered="false" :body-style="{ padding: '20px 24px' }"><div class="text-sm text-gray-500">已上架商品</div><div class="mt-3 text-2xl font-bold">{{enabledProducts}}</div><div class="mt-2 text-xs text-gray-500">正在销售的商品</div></a-card></a-col><a-col :xs="12" :md="8"><a-card :bordered="false" :body-style="{ padding: '20px 24px' }"><div class="text-sm text-gray-500">可售卡密库存</div><div class="mt-3 text-2xl font-bold">{{totalStock}}</div><div class="mt-2 text-xs text-gray-500">可供订单发货的库存</div></a-card></a-col>
  </a-row>
  <a-tabs v-model:activeKey="activeTab" class="mb-2"><a-tab-pane key="products" tab="商品与库存"/><a-tab-pane key="categories" tab="商品分类"/></a-tabs>
  <a-card v-if="activeTab==='categories'" :bordered="false" :body-style="{ padding: '20px 24px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3"><div class="min-w-0"><div class="text-base font-semibold">商品分类</div><div class="mt-2 text-xs text-gray-500">分类供所有 Telegram 机器人共享使用</div></div><a-button @click="handleAddCategory">新增分类</a-button></div></template>
   <a-space v-if="categories.length" wrap><a-tag v-for="category in categories" :key="category.id" closable @close.prevent="removeCategory(category)">{{category.name}}</a-tag></a-space><a-empty v-else description="尚未创建分类"/>
  </a-card>
  <a-card v-if="activeTab==='products'" :bordered="false" :body-style="{ padding: '20px 24px' }">
   <template #title><div class="flex min-w-0 flex-wrap items-center justify-between gap-3"><div class="min-w-0"><div class="text-base font-semibold">商品与卡密库存</div><div class="mt-2 text-xs text-gray-500">卡密入库后使用 AES-256-GCM 加密保存；重复卡密会自动跳过</div></div><a-button type="primary" @click="handleAddProduct">新增商品</a-button></div></template>
   <div v-if="isMobile" class="tg-mobile-list">
    <a-card v-for="item in products" :key="item.id" size="small" :body-style="{ padding: '20px 24px' }"><div class="flex min-w-0 items-start justify-between gap-2"><div class="min-w-0 flex-1"><div class="break-words font-semibold">{{item.name}}</div><div class="mt-1 text-xs text-gray-500">{{item.categoryName||'未分类'}}</div></div><a-tag class="shrink-0" :color="item.status===1?'success':'default'">{{item.status===1?'上架':'下架'}}</a-tag></div>
     <div class="mt-3 grid grid-cols-2 gap-3"><div><div class="text-xs text-gray-500">售价</div><div class="mt-1 text-base font-bold">{{money(item)}}</div></div><div><div class="text-xs text-gray-500">可售库存</div><div class="mt-1 text-base font-bold">{{item.stockCount}}</div></div></div>
     <div class="mt-3 flex flex-wrap justify-end gap-2"><a-button size="small" @click="openImport(item)">导入卡密</a-button><a-button size="small" @click="openEdit(item)">编辑</a-button><a-button size="small" danger @click="removeProduct(item)">删除</a-button></div>
    </a-card><a-empty v-if="!loading&&products.length===0" description="还没有商品，先创建商品再导入卡密"/>
   </div>
   <a-table v-else :data-source="products" :loading="loading" row-key="id" :scroll="{x:900}">
    <a-table-column title="商品名称" data-index="name" key="name"/><a-table-column title="分类" data-index="categoryName" key="categoryName"><template #default="{record}">{{record.categoryName||'未分类'}}</template></a-table-column>
    <a-table-column title="售价" key="price"><template #default="{record}">{{money(record)}}</template></a-table-column><a-table-column title="可售库存" data-index="stockCount" key="stockCount"/><a-table-column title="状态" key="status"><template #default="{record}"><a-tag :color="record.status===1?'success':'default'">{{record.status===1?'上架':'下架'}}</a-tag></template></a-table-column>
    <a-table-column title="操作" key="actions" fixed="right" width="220"><template #default="{record}"><a-space><a-button type="link" size="small" @click="openImport(record)">导入卡密</a-button><a-button type="link" size="small" @click="openEdit(record)">编辑</a-button><a-button type="link" danger size="small" @click="removeProduct(record)">删除</a-button></a-space></template></a-table-column>
    <template #emptyText><a-empty description="还没有商品，先创建一个商品再导入卡密"/></template>
   </a-table>
  </a-card>
  <a-modal v-model:open="categoryModalOpen" title="新增商品分类" :confirm-loading="categorySaving" :width="480" @ok="saveCategory"><div class="p-6"><CategoryForm/></div></a-modal>
  <a-modal v-model:open="productModalOpen" title="商品信息" :confirm-loading="saving" :width="560" @ok="saveProduct"><div class="p-6"><ProductForm/></div></a-modal>
  <a-modal v-model:open="importModalOpen" title="导入卡密" :confirm-loading="saving" :width="560" @ok="submitImport"><div class="p-6"><a-alert class="mb-4" type="info" show-icon message="每行一条卡密，系统会在入库前加密并自动去重"/><ImportForm/></div></a-modal>
 </div>
</template>
