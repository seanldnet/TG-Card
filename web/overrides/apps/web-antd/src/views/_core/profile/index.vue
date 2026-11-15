<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { message } from 'ant-design-vue';
import { requestClient } from '#/api/request';
import { useAuthStore } from '#/store';
interface AdminProfile { id:string; username:string; realName:string; roles:string[]; }
const loading=ref(false),saving=ref(false),authStore=useAuthStore();
const profile=reactive<AdminProfile>({id:'',username:'',realName:'',roles:[]});
const password=reactive({currentPassword:'',newPassword:'',confirmPassword:''});
async function loadProfile(){loading.value=true;try{Object.assign(profile,await requestClient.get<AdminProfile>('/user/info'));}catch{message.error('个人资料加载失败');}finally{loading.value=false;}}
async function changePassword(){
 if(!password.currentPassword||password.newPassword.length<12){message.warning('请输入当前密码，新密码至少 12 位');return;}
 if(password.newPassword!==password.confirmPassword){message.warning('两次输入的新密码不一致');return;}
 saving.value=true;try{await requestClient.post('/admin/password',{currentPassword:password.currentPassword,newPassword:password.newPassword});message.success('密码修改成功，请使用新密码重新登录');await authStore.logout();}catch(error:any){message.error(error?.response?.data?.message||'密码修改失败');}finally{saving.value=false;}
}
onMounted(loadProfile);
</script>
<template>
 <div class="mx-auto w-full max-w-4xl space-y-4 p-4 md:p-6">
  <a-card :bordered="false" :loading="loading" title="个人中心">
   <a-descriptions bordered :column="{xs:1,sm:2}">
    <a-descriptions-item label="登录账号">{{profile.username||'-'}}</a-descriptions-item>
    <a-descriptions-item label="显示名称">{{profile.realName||'-'}}</a-descriptions-item>
    <a-descriptions-item label="账户角色"><a-tag v-for="role in profile.roles" :key="role" color="blue">{{role}}</a-tag></a-descriptions-item>
    <a-descriptions-item label="账户状态"><a-tag color="success">正常</a-tag></a-descriptions-item>
   </a-descriptions>
  </a-card>
  <a-card :bordered="false" title="修改登录密码">
   <a-alert class="mb-4" type="info" show-icon message="修改成功后，当前登录会话将失效，需要使用新密码重新登录。"/>
   <a-form layout="vertical" class="w-full max-w-xl" @finish="changePassword">
    <a-form-item label="当前密码" required><a-input-password v-model:value="password.currentPassword" autocomplete="current-password" placeholder="请输入当前密码"/></a-form-item>
    <a-form-item label="新密码" required><a-input-password v-model:value="password.newPassword" autocomplete="new-password" placeholder="至少 12 位"/></a-form-item>
    <a-form-item label="确认新密码" required><a-input-password v-model:value="password.confirmPassword" autocomplete="new-password" placeholder="再次输入新密码"/></a-form-item>
    <a-button type="primary" html-type="submit" :loading="saving">保存新密码</a-button>
   </a-form>
  </a-card>
 </div>
</template>
