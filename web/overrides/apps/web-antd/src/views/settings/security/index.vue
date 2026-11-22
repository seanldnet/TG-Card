<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { $t } from '#/locales';
import { useAuthStore } from '#/store';
const authStore=useAuthStore();

const loading=ref(false),enabled=ref(false),enabledAt=ref('');
const enrollment=ref(false),secret=ref(''),provisioningUri=ref('');
const recoveryCodes=ref<string[]>([]);
const currentPassword=ref(''),confirmCode=ref(''),disablePassword=ref(''),disableCode=ref('');
async function loadStatus(){
  loading.value=true;
  try{const result=await requestClient.get<{enabled:boolean;enabledAt:string}>('/admin/security/totp/status');enabled.value=result.enabled;enabledAt.value=result.enabledAt||'';}
  catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.securitySettings.statusLoadFailed'));}
  finally{loading.value=false;}
}
async function startEnrollment(){
  if(!currentPassword.value){message.warning($t('tg-card.securitySettings.currentPasswordRequired'));return;}
  loading.value=true;
  try{
    const result=await requestClient.post<{secret:string;provisioningUri:string}>('/admin/security/totp/enroll',{password:currentPassword.value});
    secret.value=result.secret;provisioningUri.value=result.provisioningUri;enrollment.value=true;confirmCode.value='';
    message.success($t('tg-card.securitySettings.secretGenerated'));
  }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.securitySettings.enrollmentFailed'));}
  finally{loading.value=false;}
}
async function confirmEnrollment(){
  if(!/^\d{6}$/.test(confirmCode.value)){message.warning($t('tg-card.securitySettings.totpRequired'));return;}
  loading.value=true;
  try{
    const result=await requestClient.post<{enabled:boolean;recoveryCodes:string[]}>('/admin/security/totp/confirm',{code:confirmCode.value});
    enabled.value=true;recoveryCodes.value=result.recoveryCodes||[];enrollment.value=false;secret.value='';provisioningUri.value='';currentPassword.value='';confirmCode.value='';
    message.success($t('tg-card.securitySettings.enabledSuccess'));
  }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.securitySettings.confirmFailed'));}
  finally{loading.value=false;}
}
async function disableTOTP(){
  if(!disablePassword.value||!/^\d{6}$/.test(disableCode.value)){message.warning($t('tg-card.securitySettings.disableRequired'));return;}
  loading.value=true;
  try{
    await requestClient.post('/admin/security/totp/disable',{password:disablePassword.value,code:disableCode.value});
    enabled.value=false;enabledAt.value='';disablePassword.value='';disableCode.value='';
    message.success($t('tg-card.securitySettings.disabledSuccess'));
    await authStore.logout();
  }catch(error:any){message.error(error?.response?.data?.message||$t('tg-card.securitySettings.disableFailed'));}
  finally{loading.value=false;}
}
async function finishRecoveryCodeSetup(){recoveryCodes.value=[];message.info($t('tg-card.securitySettings.reloginWithNewCode'));await authStore.logout();}
async function copyValue(value:string){try{await navigator.clipboard.writeText(value);message.success($t('tg-card.securitySettings.copied'));}catch{message.warning($t('tg-card.securitySettings.copyFailed'));}}
onMounted(loadStatus);
</script>

<template>
  <div class="mx-auto flex w-full min-w-0 max-w-4xl flex-col gap-4 p-3 md:p-6">
    <a-card :bordered="false" :title="$t('tg-card.securitySettings.title')" :loading="loading">
      
      <div v-if="enabled" class="flex flex-col gap-3">
        <div v-if="recoveryCodes.length" class="rounded-lg border border-amber-300 p-3">
          <a-alert class="mb-3" type="warning" show-icon :message="$t('tg-card.securitySettings.recoveryWarning')" />
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2"><div v-for="code in recoveryCodes" :key="code" class="rounded bg-gray-100 p-2 font-mono text-sm">{{ code }}</div></div>
          <div class="mt-3 flex flex-wrap gap-2">
            <a-button @click="copyValue(recoveryCodes.join('\n'))">{{$t('tg-card.securitySettings.copyAllRecovery')}}</a-button>
            <a-button type="primary" @click="finishRecoveryCodeSetup">{{$t('tg-card.securitySettings.savedRelogin')}}</a-button>
          </div>
        </div>
        <a-tag color="success">{{$t('tg-card.securitySettings.enabled')}}</a-tag>
        <div class="text-sm text-gray-500">{{$t('tg-card.securitySettings.enabledAt')}}：{{ enabledAt || '—' }}</div>
        <a-divider />
        <div class="font-medium">{{$t('tg-card.securitySettings.disableTitle')}}</div>
        <a-form layout="vertical">
          <a-form-item :label="$t('tg-card.securitySettings.currentPassword')"><a-input-password v-model:value="disablePassword" autocomplete="current-password" :placeholder="$t('tg-card.securitySettings.currentPasswordPlaceholder')"/></a-form-item>
          <a-form-item :label="$t('tg-card.securitySettings.totpCode')"><a-input v-model:value="disableCode" inputmode="numeric" maxlength="6" :placeholder="$t('tg-card.securitySettings.totpCodePlaceholder')"/></a-form-item>
        </a-form>
        <div><a-button danger :loading="loading" @click="disableTOTP">{{$t('tg-card.securitySettings.verifyDisable')}}</a-button></div>
      </div>
      <div v-else-if="!enrollment" class="flex flex-col gap-3">
        <a-tag color="warning">{{$t('tg-card.securitySettings.notEnabled')}}</a-tag>
        <p class="text-sm text-gray-500">{{$t('tg-card.securitySettings.setupDescription')}}</p>
        <a-form layout="vertical">
          <a-form-item :label="$t('tg-card.securitySettings.currentPassword')"><a-input-password v-model:value="currentPassword" autocomplete="current-password" :placeholder="$t('tg-card.securitySettings.currentPasswordPlaceholder')"/></a-form-item>
        </a-form>
        <div><a-button type="primary" :loading="loading" @click="startEnrollment">{{$t('tg-card.securitySettings.startEnrollment')}}</a-button></div>
      </div>
      <div v-else class="flex flex-col gap-3">
        <a-alert type="warning" show-icon :message="$t('tg-card.securitySettings.enrollmentWarning')" />
        <div class="rounded-lg border p-3">
          <div class="mb-2 text-xs text-gray-500">{{$t('tg-card.securitySettings.manualSecret')}}</div>
          <div class="break-all font-mono text-base font-semibold">{{ secret }}</div>
          <a-button class="mt-2" size="small" @click="copyValue(secret)">{{$t('tg-card.securitySettings.copySecret')}}</a-button>
        </div>
        <div class="rounded-lg border p-3">
          <div class="mb-2 text-xs text-gray-500">{{$t('tg-card.securitySettings.provisioningUri')}}</div>
          <div class="break-all text-xs">{{ provisioningUri }}</div>
          <a-button class="mt-2" size="small" @click="copyValue(provisioningUri)">{{$t('tg-card.securitySettings.copyUri')}}</a-button>
        </div>
        <a-form layout="vertical">
          <a-form-item :label="$t('tg-card.securitySettings.codeLabel')"><a-input v-model:value="confirmCode" inputmode="numeric" maxlength="6" :placeholder="$t('tg-card.securitySettings.codePlaceholder')"/></a-form-item>
        </a-form>
        <div class="flex flex-wrap gap-2">
          <a-button type="primary" :loading="loading" @click="confirmEnrollment">{{$t('tg-card.securitySettings.confirmEnable')}}</a-button>
          <a-button :disabled="loading" @click="enrollment=false;secret='';provisioningUri='';confirmCode=''">{{$t('tg-card.securitySettings.cancelEnrollment')}}</a-button>
        </div>
      </div>
    </a-card>
  </div>
</template>
