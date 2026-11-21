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
  // Vben's upstream implementation uses baseRequestClient, which omits the
  // Authorization header. Use requestClient so the API can revoke this
  // browser's short-lived access session as well as its HttpOnly refresh family.
  return requestClient.post('/auth/logout', undefined, {
    withCredentials: true,
  });
}

export async function getAccessCodesApi() {
  return requestClient.get<string[]>('/auth/codes');
}
