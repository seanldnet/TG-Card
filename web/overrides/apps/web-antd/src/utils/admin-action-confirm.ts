import { h } from 'vue';

import { Input, Modal, message } from 'ant-design-vue';

import { requestClient } from '#/api/request';

type ActionMethod = 'POST' | 'PUT' | 'DELETE';

interface ConfirmedActionOptions<T> {
  actionKey: string;
  method: ActionMethod;
  path: string;
  payload?: unknown;
  run: (confirmationToken?: string) => Promise<T>;
}

function requestStepUpToken(options: ConfirmedActionOptions<unknown>, totpRequired: boolean) {
  return new Promise<string>((resolve, reject) => {
    let password = '';
    let code = '';

    Modal.confirm({
      title: '敏感操作二次确认',
      content: h('div', { class: 'space-y-3' }, [
        h('p', { class: 'text-sm text-gray-600' }, '为保护账户安全，请再次输入管理员密码后继续此操作。'),
        h(Input.Password, {
          autofocus: true,
          autocomplete: 'current-password',
          placeholder: '管理员当前密码',
          onChange: (event: Event) => {
            password = (event.target as HTMLInputElement).value;
          },
        }),
        ...(totpRequired
          ? [
              h(Input, {
                autocomplete: 'one-time-code',
                placeholder: '6 位动态验证码或一次性恢复码',
                onChange: (event: Event) => {
                  code = (event.target as HTMLInputElement).value;
                },
              }),
            ]
          : []),
      ]),
      okText: '验证并继续',
      cancelText: '取消',
      onCancel: () => reject(new Error('已取消敏感操作')),
      onOk: async () => {
        if (!password.trim()) {
          message.warning('请输入管理员当前密码');
          return Promise.reject(new Error('管理员密码不能为空'));
        }
        if (totpRequired && !code.trim()) {
          message.warning('请输入动态验证码或一次性恢复码');
          return Promise.reject(new Error('需要双重验证代码'));
        }
        try {
          const result = await requestClient.post<{ token: string }>('/admin/security/confirm-action', {
            password,
            ...(totpRequired ? { totpCode: code } : {}),
            actionKey: options.actionKey,
            method: options.method,
            path: options.path,
            ...(options.payload === undefined ? {} : { payload: options.payload }),
          });
          if (!result?.token) throw new Error('服务器未返回二次确认凭证');
          resolve(result.token);
        } catch (error: any) {
          const text = error?.response?.data?.message || error?.message || '二次确认失败，请重试';
          message.error(text);
          return Promise.reject(error);
        }
      },
    });
  });
}

export async function withAdminActionConfirmation<T>(options: ConfirmedActionOptions<T>): Promise<T> {
  try {
    return await options.run();
  } catch (error: any) {
    if (error?.response?.status !== 428) throw error;
    const details = error?.response?.data?.data || {};
    const token = await requestStepUpToken(options as ConfirmedActionOptions<unknown>, !!details.totpRequired);
    return options.run(token);
  }
}
