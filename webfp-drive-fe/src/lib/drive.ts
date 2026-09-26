import type { DragEvent } from 'react';

import { apiRequest, clearAPISession } from './api';

export type DriveFile = {
  id: string;
  owner_id: string;
  folder_id?: string | null;
  filename: string;
  content_type: string;
  size_bytes: number;
  checksum_sha256?: string | null;
  status: string;
  created_at: string;
  updated_at: string;
};

export type DriveFolder = {
  id: string;
  owner_id: string;
  parent_id?: string | null;
  name: string;
  created_at: string;
  updated_at: string;
};

export type ShareLink = {
  id: string;
  expires_at: string;
  url: string;
};

export type ShareListItem = {
  id: string;
  file_id: string;
  filename: string;
  url: string;
  expires_at: string;
  max_downloads: number;
  download_count: number;
  has_password: boolean;
};

export type StorageUsage = {
  used_bytes: number;
  limit_bytes: number;
  file_count: number;
};

export type DragPayload = {
  kind: 'file' | 'folder';
  id: string;
};

export const DRAG_MIME = 'application/x-webfp-item';

async function request(path: string, init?: RequestInit): Promise<Response> {
  const response = await apiRequest(path, init);
  if (response.status === 401) {
    clearAPISession();
    window.location.assign('/auth/sign-in');
    throw new Error('Your session has expired. Please sign in again.');
  }
  return response;
}

async function expect(response: Response, fallback: string): Promise<Response> {
  if (!response.ok) {
    let message = fallback;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // keep the fallback message when the body is not JSON
    }
    throw new Error(message);
  }
  return response;
}

function scoped(path: string, param: string, id?: string | null): string {
  return id ? `${path}?${param}=${encodeURIComponent(id)}` : path;
}

export async function listFolders(parentId?: string | null): Promise<DriveFolder[]> {
  const response = await expect(
    await request(scoped('/api/v1/folders', 'parent_id', parentId)),
    'Unable to load folders.',
  );
  return (await response.json()) as DriveFolder[];
}

export async function listFiles(folderId?: string | null): Promise<DriveFile[]> {
  const response = await expect(
    await request(scoped('/api/v1/files', 'folder_id', folderId)),
    'Unable to load files.',
  );
  return (await response.json()) as DriveFile[];
}

export async function getFolder(id: string): Promise<DriveFolder> {
  const response = await expect(
    await request(`/api/v1/folders/${id}`),
    'Unable to load that folder.',
  );
  return (await response.json()) as DriveFolder;
}

export async function createFolder(name: string, parentId: string | null): Promise<DriveFolder> {
  const response = await expect(
    await request('/api/v1/folders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, parent_id: parentId }),
    }),
    'Unable to create the folder.',
  );
  return (await response.json()) as DriveFolder;
}

export async function moveFolder(id: string, parentId: string | null): Promise<void> {
  await expect(
    await request(`/api/v1/folders/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ parent_id: parentId }),
    }),
    'Unable to move the folder.',
  );
}

export async function deleteFolder(id: string): Promise<void> {
  await expect(
    await request(`/api/v1/folders/${id}`, { method: 'DELETE' }),
    'Unable to delete the folder.',
  );
}

export async function moveFile(id: string, folderId: string | null): Promise<void> {
  await expect(
    await request(`/api/v1/files/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ folder_id: folderId }),
    }),
    'Unable to move the file.',
  );
}

export async function deleteFile(id: string): Promise<void> {
  await expect(await request(`/api/v1/files/${id}`, { method: 'DELETE' }), 'Unable to delete the file.');
}

export async function uploadFiles(files: File[], folderId: string | null): Promise<void> {
  for (const file of files) {
    const form = new FormData();
    form.append('file', file, file.name);
    if (folderId) form.append('folder_id', folderId);
    await expect(
      await request('/api/v1/files', { method: 'POST', body: form }),
      `Upload of "${file.name}" failed.`,
    );
  }
}

export async function createShare(
  fileId: string,
  options: { expiresAt: string; maxDownloads?: number; password?: string },
): Promise<ShareLink> {
  const body: Record<string, unknown> = { expires_at: options.expiresAt };
  if (options.maxDownloads !== undefined) body.max_downloads = options.maxDownloads;
  if (options.password) body.password = options.password;
  const response = await expect(
    await request(`/api/v1/files/${fileId}/shares`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),
    'Unable to create the share link.',
  );
  return (await response.json()) as ShareLink;
}

export async function getStorageUsage(): Promise<StorageUsage> {
  const response = await expect(await request('/api/v1/users/me/usage'), 'Unable to load storage usage.');
  return (await response.json()) as StorageUsage;
}

export async function listShares(): Promise<ShareListItem[]> {
  const response = await expect(await request('/api/v1/shares'), 'Unable to load your share links.');
  return (await response.json()) as ShareListItem[];
}

export async function revokeShare(id: string): Promise<void> {
  await expect(
    await request(`/api/v1/shares/${id}`, { method: 'DELETE' }),
    'Unable to revoke that share link.',
  );
}

export async function downloadFile(file: DriveFile): Promise<void> {
  const response = await expect(
    await request(`/api/v1/files/${file.id}/download`),
    `Download of "${file.filename}" failed.`,
  );
  const objectURL = URL.createObjectURL(await response.blob());
  const anchor = document.createElement('a');
  anchor.href = objectURL;
  anchor.download = file.filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(objectURL), 1_000);
}

export async function walkCrumbs(folderId: string): Promise<DriveFolder[]> {
  const chain: DriveFolder[] = [];
  const seen = new Set<string>();
  let current: string | null = folderId;
  while (current && !seen.has(current)) {
    seen.add(current);
    const folder = await getFolder(current);
    chain.unshift(folder);
    current = folder.parent_id ?? null;
  }
  return chain;
}

export function parseDragPayload(raw: string): DragPayload | null {
  try {
    const parsed = JSON.parse(raw) as Partial<DragPayload>;
    if ((parsed.kind === 'file' || parsed.kind === 'folder') && parsed.id) {
      return { kind: parsed.kind, id: parsed.id };
    }
  } catch {
    return null;
  }
  return null;
}

export function isFileDrag(event: DragEvent): boolean {
  return Array.from(event.dataTransfer.types).includes('Files');
}

export function isItemDrag(event: DragEvent): boolean {
  return Array.from(event.dataTransfer.types).includes(DRAG_MIME);
}
