const apiBaseURL = (import.meta.env.VITE_API_URL ?? '').replace(/\/+$/, '');

type AuthSession = {
  token: string;
  refresh_token: string;
};

async function readJSON<T>(response: Response): Promise<T> {
  return (await response.json()) as T;
}

type Credentials = {
  email: string;
  password: string;
  name?: string;
};

async function startSession(path: string, credentials: Credentials): Promise<void> {
  const response = await fetch(`${apiBaseURL}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(credentials),
  });
  const result = await readJSON<Partial<AuthSession> & { error?: string }>(
    response,
  ).catch(() => ({}) as Partial<AuthSession> & { error?: string });
  if (!response.ok || !result.token || !result.refresh_token) {
    throw new Error(result.error ?? `Sign-in failed (${response.status})`);
  }
  saveSession(result.token, result.refresh_token);
}

export async function signIn(email: string, password: string): Promise<void> {
  await startSession('/api/v1/auth/login', { email, password });
}

export async function register(name: string, email: string, password: string): Promise<void> {
  await startSession('/api/v1/auth/register', { email, password, name });
}

export function saveSession(token: string, refreshToken: string) {
  sessionStorage.setItem('webfp-api-token', token);
  sessionStorage.setItem('webfp-refresh-token', refreshToken);
}

export function startGitHubLogin() {
  window.location.assign(`${apiBaseURL}/api/v1/auth/github`);
}

export async function apiRequest(path: string, init: RequestInit = {}) {
  const request = (token: string | null) => {
    const headers = new Headers(init.headers);
    if (token) headers.set('Authorization', `Bearer ${token}`);
    return fetch(`${apiBaseURL}${path}`, { ...init, headers });
  };
  let response = await request(sessionStorage.getItem('webfp-api-token'));
  const refreshToken = sessionStorage.getItem('webfp-refresh-token');
  if (response.status !== 401 || !refreshToken) return response;

  const refreshResponse = await fetch(`${apiBaseURL}/api/v1/auth/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  const refreshed = await readJSON<AuthSession | { error?: string }>(refreshResponse);
  if (!refreshResponse.ok || !('token' in refreshed) || !('refresh_token' in refreshed)) {
    clearAPISession();
    return response;
  }
  sessionStorage.setItem('webfp-api-token', refreshed.token);
  sessionStorage.setItem('webfp-refresh-token', refreshed.refresh_token);
  response = await request(refreshed.token);
  return response;
}

export function clearAPISession() {
  sessionStorage.removeItem('webfp-api-token');
  sessionStorage.removeItem('webfp-refresh-token');
}
