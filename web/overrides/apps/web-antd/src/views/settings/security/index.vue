<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { useAuthStore } from '#/store';
const authStore=useAuthStore();

const loading=ref(false),enabled=ref(false),enabledAt=ref('');
const enrollment=ref(false),secret=ref(''),provisioningUri=ref('');
const recoveryCodes=ref<string[]>([]);
const currentPassword=ref(''),confirmCode=ref(''),disablePassword=ref(''),disableCode=ref('');
async function loadStatus(){
  loading.value=true;
  try{const result=await requestClient.get<{enabled:boolean;enabledAt:string}>('/admin/security/totp/status');enabled.value=result.enabled;enabledAt.value=result.enabledAt||'';}
  catch(error:any){message.error(error?.response?.data?.message||'读取双重验证状态失败');}
  finally{loading.value=false;}
}
async function startEnrollment(){
  if(!currentPassword.value){message.warning('请输入当前登录密码');return;}
  loading.value=true;
  try{
    const result=await requestClient.post<{secret:string;provisioningUri:string}>('/admin/security/totp/enroll',{password:currentPassword.value});
    secret.value=result.secret;provisioningUri.value=result.provisioningUri;enrollment.value=true;confirmCode.value='';
    message.success('绑定密钥已生成，请先添加到验证器应用');
  }catch(error:any){message.error(error?.response?.data?.message||'开始绑定失败');}
  finally{loading.value=false;}
}
async function confirmEnrollment(){
  if(!/^\d{6}$/.test(confirmCode.value)){message.warning('请输入验证器当前显示的 6 位验证码');return;}
  loading.value=true;
  try{
    const result=await requestClient.post<{enabled:boolean;recoveryCodes:string[]}>('/admin/security/totp/confirm',{code:confirmCode.value});
    enabled.value=true;recoveryCodes.value=result.recoveryCodes||[];enrollment.value=false;secret.value='';provisioningUri.value='';currentPassword.value='';confirmCode.value='';
    message.success('管理员双重验证已启用，请立即保存一次性恢复码');
  }catch(error:any){message.error(error?.response?.data?.message||'验证码确认失败');}
  finally{loading.value=false;}
}
async function disableTOTP(){
  if(!disablePassword.value||!/^\d{6}$/.test(disableCode.value)){message.warning('请输入当前密码和 6 位动态验证码');return;}
  loading.value=true;
  try{
    await requestClient.post('/admin/security/totp/disable',{password:disablePassword.value,code:disableCode.value});
    enabled.value=false;enabledAt.value='';disablePassword.value='';disableCode.value='';
    message.success('双重验证已关闭，请重新登录');
    await authStore.logout();
  }catch(error:any){message.error(error?.response?.data?.message||'关闭双重验证失败');}
  finally{loading.value=false;}
}
async function finishRecoveryCodeSetup(){recoveryCodes.value=[];message.info('请使用新动态验证码重新登录');await authStore.logout();}
async function copyValue(value:string){try{await navigator.clipboard.writeText(value);message.success('已复制');}catch{message.warning('复制失败，请手动选择文本复制');}}
onMounted(loadStatus);
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-4xl flex-col gap-4 p-3 md:p-6">
    <a-card :bordered="false" title="管理员双重验证（TOTP）" :loading="loading">
      <a-alert class="mb-4" type="info" show-icon message="启用后，管理员每次登录除了密码，还必须输入验证器应用生成的 6 位动态验证码。请使用支持 TOTP 的验证器应用，并妥善保管绑定密钥。" />
      <div v-if="enabled" class="flex flex-col gap-3">
        <div v-if="recoveryCodes.length" class="rounded-lg border border-amber-300 p-3">
          <a-alert class="mb-3" type="warning" show-icon message="这些一次性恢复码只显示这一次。每个恢复码只能使用一次，请复制并保存在密码管理器等安全位置；关闭页面后无法再次查看。" />
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2"><div v-for="code in recoveryCodes" :key="code" class="rounded bg-gray-100 p-2 font-mono text-sm">{{ code }}</div></div>
          <div class="mt-3 flex flex-wrap gap-2">
            <a-button @click="copyValue(recoveryCodes.join('\n'))">复制全部恢复码</a-button>
            <a-button type="primary" @click="finishRecoveryCodeSetup">已保存，重新登录</a-button>
          </div>
        </div>
        <a-tag color="success">双重验证已启用</a-tag>
        <div class="text-sm text-gray-500">启用时间：{{ enabledAt || '—' }}</div>
        <a-divider />
        <div class="font-medium">关闭双重验证</div>
        <a-form layout="vertical">
          <a-form-item label="当前登录密码"><a-input-password v-model:value="disablePassword" autocomplete="current-password" placeholder="请输入当前密码"/></a-form-item>
          <a-form-item label="动态验证码"><a-input v-model:value="disableCode" inputmode="numeric" maxlength="6" placeholder="请输入验证器的 6 位验证码"/></a-form-item>
        </a-form>
        <div><a-button danger :loading="loading" @click="disableTOTP">验证并关闭双重验证</a-button></div>
      </div>
      <div v-else-if="!enrollment" class="flex flex-col gap-3">
        <a-tag color="warning">双重验证尚未启用</a-tag>
        <p class="text-sm text-gray-500">绑定前需要再次验证当前密码。系统会生成一次性绑定密钥，验证器成功校验后才会正式启用。</p>
        <a-form layout="vertical">
          <a-form-item label="当前登录密码"><a-input-password v-model:value="currentPassword" autocomplete="current-password" placeholder="请输入当前密码"/></a-form-item>
        </a-form>
        <div><a-button type="primary" :loading="loading" @click="startEnrollment">开始绑定验证器</a-button></div>
      </div>
      <div v-else class="flex flex-col gap-3">
        <a-alert type="warning" show-icon message="请先在验证器应用中添加账户，再输入应用当前显示的验证码完成绑定。密钥只在本次绑定过程中显示。" />
        <div class="rounded-lg border p-3">
          <div class="mb-2 text-xs text-gray-500">手动输入密钥</div>
          <div class="break-all font-mono text-base font-semibold">{{ secret }}</div>
          <a-button class="mt-2" size="small" @click="copyValue(secret)">复制密钥</a-button>
        </div>
        <div class="rounded-lg border p-3">
          <div class="mb-2 text-xs text-gray-500">验证器配置 URI</div>
          <div class="break-all text-xs">{{ provisioningUri }}</div>
          <a-button class="mt-2" size="small" @click="copyValue(provisioningUri)">复制 URI</a-button>
        </div>
        <a-form layout="vertical">
          <a-form-item label="验证器 6 位动态验证码"><a-input v-model:value="confirmCode" inputmode="numeric" maxlength="6" placeholder="输入当前显示的 6 位验证码"/></a-form-item>
        </a-form>
        <div class="flex flex-wrap gap-2">
          <a-button type="primary" :loading="loading" @click="confirmEnrollment">确认并启用</a-button>
          <a-button :disabled="loading" @click="enrollment=false;secret='';provisioningUri='';confirmCode=''">取消绑定</a-button>
        </div>
      </div>
    </a-card>
  </div>
</template>
