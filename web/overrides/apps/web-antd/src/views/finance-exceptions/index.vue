<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';

interface FinanceException {id:string;sourceKey:string;category:string;severity:string;entityType:string;entityId:string;title:string;detail:string;status:string;resolutionNote:string;resolvedBy:string;createdAt:string;updatedAt:string;resolvedAt:string}
interface UncertainOrder {localOrderId:string;localOrderNo:string;externalOrderId:string;externalOrderNo:string;status:string;lastError:string;createdAt:string;ageingSeconds:number;waitSeconds:number}
interface FundCheck {localOrderId:string;localOrderNo:string;externalOrderId:string;currency:string;balanceBefore:string;balanceAfter:string;expectedDebit:string;observedDebit:string;difference:string;status:string;detail:string;checkedAt:string}
const exceptions=ref<FinanceException[]>([]),uncertain=ref<UncertainOrder[]>([]),fundChecks=ref<FundCheck[]>([]);
const openCount=ref(0),loading=ref(false),checking=ref(false),statusFilter=ref('open');
const selected=ref<FinanceException|null>(null),note=ref(''),actionStatus=ref('resolved'),saving=ref(false),modalOpen=ref(false);
const uncertainSelected=ref<UncertainOrder|null>(null),resolution=ref('accepted'),externalOrderId=ref(''),externalOrderNo=ref(''),resolutionNote=ref(''),resolutionOpen=ref(false),resolving=ref(false);
const categoryLabel=(v:string)=>({procurement_failed:'采购失败',procurement_uncertain:'采购受理不确定',supplier_balance:'上下游余额差异',refund_ledger:'退款账本异常'} as Record<string,string>)[v]||v;
const severityColor=(v:string)=>v==='critical'?'red':v==='error'?'orange':'gold';
const severityLabel=(v:string)=>({critical:'严重',error:'错误',warning:'警告',info:'提示'} as Record<string,string>)[v]||v;
const entityTypeLabel=(v:string)=>({order:'订单',admin:'管理员',supplier:'供应商',payment:'支付',wallet:'钱包',telegram_user:'Telegram 用户'} as Record<string,string>)[v]||v;
const fundStatusLabel=(v:string)=>({matched:'一致',mismatch:'存在差异',pending:'待核对',error:'检查失败'} as Record<string,string>)[v]||v;
const statusLabel=(v:string)=>({open:'待处理',processing:'处理中',resolved:'已解决',ignored:'已忽略'} as Record<string,string>)[v]||v;
const ageing=(n:number)=>{const h=Math.floor(n/3600),d=Math.floor(h/24);return d>0?d+'天'+Math.floor((h%24))+'小时':h>0?h+'小时'+Math.floor((n%3600)/60)+'分钟':Math.floor(n/60)+'分钟';};
const openBadge=computed(()=>openCount.value>0?openCount.value:0);
async function load(){
 loading.value=true;
 try{
  const [list,count,unc,funds]=await Promise.all([
   requestClient.get<FinanceException[]>('/finance/exceptions',{params:{status:statusFilter.value,limit:200}}),
   requestClient.get<{openCount:number}>('/finance/exceptions/count'),
   requestClient.get<UncertainOrder[]>('/upstream/procurements/uncertain',{params:{limit:200}}),
   requestClient.get<FundCheck[]>('/upstream/fund-checks',{params:{limit:200}}),
  ]);
  exceptions.value=list||[];openCount.value=count?.openCount||0;uncertain.value=unc||[];fundChecks.value=funds||[];
 }catch(error:any){message.error(error?.response?.data?.message||'资金异常数据加载失败');}
 finally{loading.value=false;}
}
function openException(item:FinanceException,status='resolved'){selected.value=item;note.value='';actionStatus.value=status;modalOpen.value=true;}
async function saveException(){
 if(!selected.value)return;
 if(actionStatus.value==='resolved'&&!note.value.trim()){message.warning('请填写核实结论');return;}
 saving.value=true;
 try{await requestClient.put('/finance/exceptions/'+selected.value.id,{status:actionStatus.value,note:note.value.trim()});message.success('异常处理状态已更新');modalOpen.value=false;await load();}
 catch(error:any){message.error(error?.response?.data?.message||'更新异常状态失败');}
 finally{saving.value=false;}
}
function openUncertain(item:UncertainOrder,choice:string){uncertainSelected.value=item;resolution.value=choice;externalOrderId.value=item.externalOrderId||'';externalOrderNo.value=item.externalOrderNo||'';resolutionNote.value='';resolutionOpen.value=true;}
async function resolveUncertain(){
 if(!uncertainSelected.value)return;
 if(!resolutionNote.value.trim()){message.warning('请填写供应商核实结果');return;}
 if(resolution.value==='accepted'&&!externalOrderId.value.trim()){message.warning('请填写供应商上游订单 ID');return;}
 resolving.value=true;
 try{await requestClient.post('/upstream/procurements/'+uncertainSelected.value.localOrderId+'/resolve',{resolution:resolution.value,externalOrderId:externalOrderId.value.trim(),externalOrderNo:externalOrderNo.value.trim(),note:resolutionNote.value.trim()});message.success(resolution.value==='accepted'?'已登记上游订单，恢复查单':'已确认上游未受理，订单转采购失败待处理');resolutionOpen.value=false;await load();}
 catch(error:any){message.error(error?.response?.data?.message||'悬空订单处理失败');}
 finally{resolving.value=false;}
}
async function runRefundCheck(){
 checking.value=true;
 try{const result=await requestClient.post<{refundLedgerAnomalies:number;autoCorrection:boolean}>('/finance/refund-ledger/check',{});message.success('检查完成：发现 '+result.refundLedgerAnomalies+' 条退款账本异常；自动修正：'+(result.autoCorrection?'已执行':'未执行'));await load();}
 catch(error:any){message.error(error?.response?.data?.message||'退款账本检查失败');}
 finally{checking.value=false;}
}
onMounted(load);
</script>

<template>
 <div class="mx-auto flex w-full min-w-0 max-w-none flex-col gap-4 p-3 md:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
   <div><h1 class="m-0 text-xl font-semibold">资金异常中心</h1><p class="mb-0 mt-1 text-xs text-gray-500">统一查看采购失败、悬空订单、供应商余额差异和退款账本异常。所有检查只读，不会自动修改余额或流水。</p></div>
   <div class="flex items-center gap-2"><a-badge :count="openBadge" :overflow-count="999"><a-tag color="error">待处理异常</a-tag></a-badge><a-button size="small" :loading="checking" @click="runRefundCheck">执行退款账本检查</a-button><a-button size="small" :loading="loading" @click="load">刷新</a-button></div>
  </div>
  <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
   <a-card :bordered="false"><div class="text-xs text-gray-500">待处理/处理中</div><div class="mt-2 text-2xl font-semibold">{{openCount}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">悬空采购订单</div><div class="mt-2 text-2xl font-semibold">{{uncertain.length}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">供应商资金差异</div><div class="mt-2 text-2xl font-semibold">{{fundChecks.filter(x=>x.status==='mismatch').length}}</div></a-card>
   <a-card :bordered="false"><div class="text-xs text-gray-500">退款账本异常</div><div class="mt-2 text-2xl font-semibold">{{exceptions.filter(x=>x.category==='refund_ledger').length}}</div></a-card>
  </div>
  <a-card :bordered="false" title="统一异常列表" :loading="loading">
   <template #extra><a-select v-model:value="statusFilter" class="w-32" @change="load"><a-select-option value="open">待处理</a-select-option><a-select-option value="processing">处理中</a-select-option><a-select-option value="resolved">已解决</a-select-option><a-select-option value="ignored">已忽略</a-select-option></a-select></template>
   <a-table :data-source="exceptions" row-key="id" :pagination="{pageSize:10}" :scroll="{x:1100}">
    <a-table-column title="级别" key="severity" :width="90"><template #default="{record}"><a-tag :color="severityColor(record.severity)">{{severityLabel(record.severity)}}</a-tag></template></a-table-column>
    <a-table-column title="分类" key="category" :width="140"><template #default="{record}">{{categoryLabel(record.category)}}</template></a-table-column>
    <a-table-column title="异常事项" data-index="title" :width="220"/>
    <a-table-column title="关联对象" key="entity" :width="180"><template #default="{record}">{{entityTypeLabel(record.entityType)}} · {{record.entityId}}</template></a-table-column>
    <a-table-column title="详情" data-index="detail" :ellipsis="true" :width="320"/>
    <a-table-column title="状态" key="status" :width="110"><template #default="{record}"><a-tag>{{statusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="发现时间" data-index="createdAt" :width="190"/>
    <a-table-column title="处理" key="actions" :fixed="'right'" :width="170"><template #default="{record}"><div class="flex gap-1"><a-button size="small" @click="openException(record,'processing')">处理中</a-button><a-button size="small" type="primary" @click="openException(record,'resolved')">完成</a-button></div></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&!exceptions.length" description="当前筛选下没有异常"/>
  </a-card>
  <a-card :bordered="false" title="待核实采购订单" :loading="loading">
   <a-alert class="mb-3" type="warning" show-icon message="悬空订单表示采购请求已发出但无法确认供应商是否受理。必须先在供应商后台按本地订单号核实；确认已受理后录入上游订单 ID 恢复查单，确认未受理后才可使用原幂等号重试或退款。"/>
   <a-table :data-source="uncertain" row-key="localOrderId" :pagination="{pageSize:8}" :scroll="{x:1100}">
    <a-table-column title="本地订单号" data-index="localOrderNo" :width="190"/>
    <a-table-column title="上游订单 ID" data-index="externalOrderId" :width="130"/>
    <a-table-column title="等待时长" key="age" :width="130"><template #default="{record}">{{ageing(record.ageingSeconds)}}</template></a-table-column>
    <a-table-column title="异常原因" data-index="lastError" :width="320"/>
    <a-table-column title="操作" :fixed="'right'" :width="210"><template #default="{record}"><div class="flex gap-1"><a-button size="small" type="primary" @click="openUncertain(record,'accepted')">上游已受理</a-button><a-button size="small" danger @click="openUncertain(record,'not_accepted')">确认未受理</a-button></div></template></a-table-column>
   </a-table>
   <a-empty v-if="!loading&&!uncertain.length" description="没有悬空订单"/>
  </a-card>
  <a-card :bordered="false" title="上下游采购资金对账" :loading="loading">
   <a-alert class="mb-3" type="info" show-icon message="采购前后分别读取供应商余额快照，以本地订单号关联上游订单金额与余额变动。供应商同期其他操作可能影响余额差值；所有差异均进入人工核查，不自动冲账。"/>
   <a-table :data-source="fundChecks" row-key="localOrderId" :pagination="{pageSize:8}" :scroll="{x:1250}">
    <a-table-column title="本地订单号" data-index="localOrderNo" :width="190"/>
    <a-table-column title="上游订单 ID" data-index="externalOrderId" :width="130"/>
    <a-table-column title="币种" data-index="currency" :width="80"/>
    <a-table-column title="采购金额" data-index="expectedDebit" :width="120"/>
    <a-table-column title="余额变动" data-index="observedDebit" :width="120"/>
    <a-table-column title="差额" data-index="difference" :width="110"/>
    <a-table-column title="结果" key="status" :width="120"><template #default="{record}"><a-tag :color="record.status==='matched'?'success':record.status==='mismatch'?'error':'warning'">{{fundStatusLabel(record.status)}}</a-tag></template></a-table-column>
    <a-table-column title="说明" data-index="detail" :width="300"/>
    <a-table-column title="核对时间" data-index="checkedAt" :width="190"/>
   </a-table>
   <a-empty v-if="!loading&&!fundChecks.length" description="暂无采购资金核对记录；新采购成功后生成快照对比"/>
  </a-card>
  <a-modal v-model:open="modalOpen" title="处理资金异常" :confirm-loading="saving" @ok="saveException">
   <div v-if="selected" class="mb-3"><div class="font-medium">{{selected.title}}</div><div class="mt-1 text-xs text-gray-500">{{selected.detail}}</div></div>
   <a-form-item label="处理状态"><a-select v-model:value="actionStatus"><a-select-option value="processing">处理中</a-select-option><a-select-option value="resolved">已解决</a-select-option><a-select-option value="ignored">忽略</a-select-option></a-select></a-form-item>
   <a-form-item label="核实说明"><a-textarea v-model:value="note" :rows="3" :maxlength="500" show-count placeholder="填写核实结论、证据或后续处理记录"/></a-form-item>
  </a-modal>
  <a-modal v-model:open="resolutionOpen" title="Owner 人工核实悬空订单" :confirm-loading="resolving" @ok="resolveUncertain">
   <div v-if="uncertainSelected" class="mb-3 text-sm">本地订单：{{uncertainSelected.localOrderNo}} · 等待 {{ageing(uncertainSelected.ageingSeconds)}}</div>
   <a-form-item label="核实结果"><a-radio-group v-model:value="resolution"><a-radio value="accepted">供应商已受理</a-radio><a-radio value="not_accepted">供应商确认未受理</a-radio></a-radio-group></a-form-item>
   <a-form-item v-if="resolution==='accepted'" label="供应商上游订单 ID"><a-input v-model:value="externalOrderId" placeholder="填写供应商订单 ID"/></a-form-item>
   <a-form-item v-if="resolution==='accepted'" label="供应商订单号（可选）"><a-input v-model:value="externalOrderNo" placeholder="填写供应商订单号"/></a-form-item>
   <a-form-item label="核实记录"><a-textarea v-model:value="resolutionNote" :rows="3" :maxlength="500" placeholder="记录在供应商后台的查询结果、时间和依据"/></a-form-item>
  </a-modal>
 </div>
</template>
