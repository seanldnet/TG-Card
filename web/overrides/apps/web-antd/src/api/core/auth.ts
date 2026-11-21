import { baseRequestClient, requestClient } from '#/api/request';

export namespace AuthApi {
  export interface LoginParams {
    password?: string;
    username?: string;
    totpCode?: string;
    totpRecoveryCode?: string;
    capToken?: string;
  }

  export interface LoginResult {
    accessToken: string;
  }

  export interface RefreshTokenResult {
    data: string;
    status: number;
  }
}

export async function loginApi(data: AuthApi.LoginParams) {
  return requestClient.post<AuthApi.LoginResult>('/auth/login', data);
}

export async function refreshTokenApi() {
  return baseRequestClient.post<AuthApi.RefreshTokenResult>(
    '/auth/refresh',
    undefined,
    { withCredentials: true },
  );
}

export async function logoutApi() {
  // Use the authenticated client so the server can revoke both the refresh
  // token family (HttpOnly cookie) and this browser's current access session.
  return requestClient.post('/auth/logout', undefined, {
    withCredentials: true,
  });
}

export async function getAccessCodesApi() {
  return requestClient.get<string[]>('/auth/codes');
}
