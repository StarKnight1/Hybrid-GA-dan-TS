const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "https://medium-promotion-character-silk.trycloudflare.com";

async function apiFetch<T>(
  path: string,
  init?: RequestInit & { token?: string }
): Promise<T> {
  const { token, ...rest } = init ?? {};
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string>),
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, { ...rest, headers });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(text || `HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

export interface ApiResponse<T> {
  data: T;
  message: string;
}

// ── Auth ─────────────────────────────────────────────────────────────────────

export async function apiLogin(identifier: string, password: string) {
  const res = await apiFetch<ApiResponse<{ token: string; role: string }>>(
    "/auth/login",
    {
      method: "POST",
      body: JSON.stringify({ identifier, password }),
    }
  );
  return res.data;
}

export async function apiGetMe(token: string) {
  const res = await apiFetch<ApiResponse<import("@/types").UserProfile>>(
    "/users/me",
    { token }
  );
  return res.data;
}

// ── Admin data ────────────────────────────────────────────────────────────────

export async function apiGetDataStatus(token: string) {
  const res = await apiFetch<ApiResponse<import("@/types").DataStatus>>(
    "/admin/data-status",
    { token }
  );
  return res.data;
}

export async function apiClearData(token: string) {
  await apiFetch("/admin/data", { method: "DELETE", token });
}

export async function apiUploadData(token: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`${API_BASE}/admin/upload`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(text || `HTTP ${res.status}`);
  }
  return res.json();
}

export function templateDownloadUrl() {
  return `${API_BASE}/admin/template`;
}

// ── Schedule generation ───────────────────────────────────────────────────────

export function buildGenerateStreamUrl(token: string, params?: Record<string, string>) {
  const q = new URLSearchParams({ ...params, token });
  return `${API_BASE}/schedule/generate/stream?${q}`;
}

// ── Saved schedules ───────────────────────────────────────────────────────────

export async function apiListSchedules(token: string) {
  const res = await apiFetch<ApiResponse<import("@/types").SavedScheduleListItem[]>>(
    "/schedules",
    { token }
  );
  return res.data;
}

export async function apiGetSchedule(token: string, id: number) {
  const res = await apiFetch<ApiResponse<import("@/types").SavedSchedule>>(
    `/schedules/${id}`,
    { token }
  );
  return res.data;
}

export async function apiSaveSchedule(
  token: string,
  title: string,
  entries: unknown[],
  meta: unknown
) {
  const res = await apiFetch<ApiResponse<{ id: number }>>("/schedules", {
    method: "POST",
    token,
    body: JSON.stringify({ title, entries, meta }),
  });
  return res.data;
}

export async function apiDeleteSchedule(token: string, id: number) {
  await apiFetch(`/schedules/${id}`, { method: "DELETE", token });
}

export async function apiDeploySchedule(token: string, id: number) {
  await apiFetch(`/schedules/${id}/deploy`, { method: "PUT", token });
}

export async function apiGetActiveSchedule(token: string) {
  const res = await apiFetch<ApiResponse<import("@/types").SavedSchedule>>(
    "/schedules/active",
    { token }
  );
  return res.data;
}

export function scheduleExportUrl(id: number) {
  return `${API_BASE}/schedules/${id}/export`;
}
