import { useCallback, useEffect, useRef, useState, Fragment, type DragEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { ConfirmDialog, NewFolderDialog, ShareDialog, SharesDialog } from '../components/dialogs';
import { KebabMenu } from '../components/KebabMenu';
import {
  ChevronRightIcon,
  DownloadIcon,
  FileIcon,
  FolderIcon,
  PlusIcon,
  ShareIcon,
  Spinner,
  TrashIcon,
  UploadIcon,
} from '../components/icons';
import { apiRequest, clearAPISession } from '../lib/api';
import {
  deleteFile,
  deleteFolder,
  downloadFile,
  getStorageUsage,
  isFileDrag,
  isItemDrag,
  listFiles,
  listFolders,
  moveFile,
  moveFolder,
  parseDragPayload,
  uploadFiles,
  walkCrumbs,
  DRAG_MIME,
  type DragPayload,
  type DriveFile,
  type DriveFolder,
  type StorageUsage,
} from '../lib/drive';
import { formatBytes, formatDate } from '../lib/format';

type User = {
  email: string;
  name: string;
};

type DeleteTarget = {
  kind: 'file' | 'folder';
  id: string;
  name: string;
};

function messageOf(cause: unknown, fallback: string): string {
  return cause instanceof Error && cause.message ? cause.message : fallback;
}

export function Drive() {
  const { folderId } = useParams<{ folderId?: string }>();
  const navigate = useNavigate();

  const [user, setUser] = useState<User | null>(null);
  const [folders, setFolders] = useState<DriveFolder[]>([]);
  const [files, setFiles] = useState<DriveFile[]>([]);
  const [crumbs, setCrumbs] = useState<Record<string, DriveFolder[]>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [uploading, setUploading] = useState(0);
  const [hover, setHover] = useState<string | null>(null);
  const [shareFile, setShareFile] = useState<DriveFile | null>(null);
  const [sharesOpen, setSharesOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [usage, setUsage] = useState<StorageUsage | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const refresh = useCallback(() => setReloadKey((key) => key + 1), []);

  useEffect(() => {
    if (!sessionStorage.getItem('webfp-api-token')) {
      navigate('/auth/sign-in', { replace: true });
      return;
    }
    void apiRequest('/api/v1/users/me')
      .then(async (response) => {
        if (response.status === 401) {
          clearAPISession();
          navigate('/auth/sign-in', { replace: true });
          return;
        }
        if (!response.ok) throw new Error('Unable to load your account.');
        setUser((await response.json()) as User);
      })
      .catch((cause: unknown) => {
        setError(messageOf(cause, 'Unable to load your account.'));
      });
  }, [navigate]);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([listFolders(folderId), listFiles(folderId), getStorageUsage()])
      .then(([folderList, fileList, storageUsage]) => {
        if (cancelled) return;
        setFolders(folderList);
        setFiles(fileList);
        setUsage(storageUsage);
        setError('');
        setLoading(false);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setError(messageOf(cause, 'Unable to load this folder.'));
        setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [folderId, reloadKey]);

  useEffect(() => {
    if (!folderId || crumbs[folderId]) return;
    let cancelled = false;
    void walkCrumbs(folderId)
      .then((chain) => {
        if (cancelled) return;
        setCrumbs((previous) => (previous[folderId] ? previous : { ...previous, [folderId]: chain }));
      })
      .catch(() => {
        // breadcrumb names are best effort; navigation still works
      });
    return () => {
      cancelled = true;
    };
  }, [folderId, crumbs]);

  const handleUpload = useCallback(
    async (chosen: FileList | File[], destination: string | null) => {
      const uploads = Array.from(chosen);
      if (uploads.length === 0) return;
      if (usage) {
        const incoming = uploads.reduce((total, file) => total + file.size, 0);
        if (usage.used_bytes + incoming > usage.limit_bytes) {
          setError(
            `Storage is full: these uploads need ${formatBytes(incoming)} but only ${formatBytes(
              Math.max(usage.limit_bytes - usage.used_bytes, 0),
            )} is left. Delete files to free space.`,
          );
          return;
        }
      }
      setUploading(uploads.length);
      try {
        await uploadFiles(uploads, destination);
        setError('');
        refresh();
      } catch (cause) {
        setError(messageOf(cause, 'Upload failed.'));
      } finally {
        setUploading(0);
      }
    },
    [refresh, usage],
  );

  const handleMove = useCallback(
    async (payload: DragPayload, destination: string | null) => {
      if (payload.kind === 'folder') {
        if (payload.id === destination || destination === folderId) return;
      } else {
        const current = files.find((file) => file.id === payload.id);
        if (current && (current.folder_id ?? null) === destination) return;
      }
      try {
        if (payload.kind === 'folder') await moveFolder(payload.id, destination);
        else await moveFile(payload.id, destination);
        setError('');
        refresh();
      } catch (cause) {
        setError(messageOf(cause, 'Unable to move that item.'));
      }
    },
    [files, folderId, refresh],
  );

  function makeZone(destination: string | null, key: string) {
    return {
      onDragOver: (event: DragEvent) => {
        if (!isFileDrag(event) && !isItemDrag(event)) return;
        event.preventDefault();
        event.stopPropagation();
        setHover(key);
      },
      onDragLeave: (event: DragEvent) => {
        if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
        event.stopPropagation();
        setHover((value) => (value === key ? null : value));
      },
      onDrop: (event: DragEvent) => {
        event.preventDefault();
        event.stopPropagation();
        setHover(null);
        if (isFileDrag(event)) {
          const dropped = event.dataTransfer.files;
          if (dropped.length > 0) void handleUpload(dropped, destination);
          return;
        }
        const payload = parseDragPayload(event.dataTransfer.getData(DRAG_MIME));
        if (payload) void handleMove(payload, destination);
      },
    };
  }

  const uploadZone = makeZone(folderId ?? null, 'upload');
  const gridZone = makeZone(folderId ?? null, 'grid');
  const rootZone = makeZone(null, 'home-crumb');

  function folderTileProps(folder: DriveFolder) {
    const zone = makeZone(folder.id, folder.id);
    return {
      zone,
      root: {
        draggable: true,
        onDragStart: (event: DragEvent) => {
          event.dataTransfer.effectAllowed = 'move';
          event.dataTransfer.setData(DRAG_MIME, JSON.stringify({ kind: 'folder', id: folder.id }));
          event.dataTransfer.setData('text/plain', folder.name);
        },
      },
    };
  }

  function fileTileProps(file: DriveFile) {
    return {
      draggable: true,
      onDragStart: (event: DragEvent) => {
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData(DRAG_MIME, JSON.stringify({ kind: 'file', id: file.id }));
        event.dataTransfer.setData('text/plain', file.filename);
      },
    };
  }

  async function signOut() {
    try {
      await apiRequest('/api/v1/auth/logout', { method: 'POST' });
    } finally {
      clearAPISession();
      navigate('/auth/sign-in', { replace: true });
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    if (deleteTarget.kind === 'file') await deleteFile(deleteTarget.id);
    else await deleteFolder(deleteTarget.id);
    refresh();
  }

  const chain = folderId ? (crumbs[folderId] ?? []) : [];
  const currentName =
    chain.length > 0 ? chain[chain.length - 1].name : folderId ? 'Folder' : 'Home';
  const isEmpty = folders.length === 0 && files.length === 0;
  const storageFull =
    usage !== null && usage.limit_bytes > 0 && usage.used_bytes >= usage.limit_bytes;
  const usagePercent =
    usage && usage.limit_bytes > 0
      ? Math.min(100, Math.round((usage.used_bytes / usage.limit_bytes) * 100))
      : 0;

  return (
    <main className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200/80 bg-white">
        <div className="mx-auto flex h-[72px] max-w-7xl items-center justify-between gap-4 px-5 sm:px-8">
          <Link className="inline-flex items-center gap-3" to="/">
            <span className="grid size-10 place-items-center rounded-xl bg-blue-700 text-white shadow-lg shadow-blue-900/15">
              <svg aria-hidden="true" className="size-5" fill="none" viewBox="0 0 24 24">
                <path
                  d="m8.5 3.75-6 10.5 3.75 6.5h12l3.25-5.75-6.5-11.25h-6.5Z"
                  stroke="currentColor"
                  strokeLinejoin="round"
                  strokeWidth="1.8"
                />
                <path d="m2.5 14.25 12.5-.5m-6.5-10 6.5 11.25" stroke="currentColor" strokeLinecap="round" strokeWidth="1.8" />
              </svg>
            </span>
            <span className="text-base font-semibold tracking-tight">WebFP Drive</span>
          </Link>
          <div className="flex items-center gap-3">
            <span className="hidden max-w-[14rem] truncate rounded-full bg-slate-100 px-3 py-1.5 text-xs font-medium text-slate-600 sm:inline-block">
              {user?.email || '…'}
            </span>
            <button
              className="inline-flex h-10 items-center gap-2 rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900"
              onClick={() => void signOut()}
              type="button"
            >
              <svg aria-hidden="true" className="size-4" fill="none" viewBox="0 0 20 20">
                <path
                  d="M8.333 5.833V4.167A1.667 1.667 0 0 1 10 2.5h5.833A1.667 1.667 0 0 1 17.5 4.167v11.666a1.667 1.667 0 0 1-1.667 1.667H10a1.667 1.667 0 0 1-1.667-1.667v-1.666M12.5 10H2.5m0 0 3.333-3.333M2.5 10l3.333 3.333"
                  stroke="currentColor"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="1.6"
                />
              </svg>
              Sign out
            </button>
          </div>
        </div>
      </header>

      <section className="mx-auto max-w-7xl px-5 pb-16 pt-8 sm:px-8">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <nav aria-label="Folder path" className="flex min-w-0 flex-wrap items-center gap-1.5 text-sm">
            <Link
              className={`rounded-lg px-2 py-1 font-semibold transition ${
                'home-crumb' === hover ? 'bg-blue-50 text-blue-700 ring-2 ring-blue-500' : 'text-slate-700 hover:bg-slate-100'
              }`}
              to="/"
              {...rootZone}
            >
              Home
            </Link>
            {chain.slice(0, -1).map((crumb) => (
              <Fragment key={crumb.id}>
                <ChevronRightIcon className="size-4 text-slate-400" />
                <Link
                  className="rounded-lg px-2 py-1 font-medium text-slate-700 transition hover:bg-slate-100"
                  to={`/f/${crumb.id}`}
                >
                  {crumb.name}
                </Link>
              </Fragment>
            ))}
            {chain.length > 0 && (
              <>
                <ChevronRightIcon className="size-4 text-slate-400" />
                <span className="max-w-[16rem] truncate rounded-lg bg-slate-100 px-2 py-1 font-semibold text-slate-900">
                  {currentName}
                </span>
              </>
            )}
          </nav>
          <div className="flex items-center gap-2.5">
            <button
              className="inline-flex h-11 items-center gap-2 rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:border-slate-300 hover:bg-slate-50"
              onClick={() => setSharesOpen(true)}
              type="button"
            >
              <ShareIcon className="size-4" />
              Share links
            </button>
            <button
              className="inline-flex h-11 items-center gap-2 rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:border-slate-300 hover:bg-slate-50"
              onClick={() => setNewFolderOpen(true)}
              type="button"
            >
              <PlusIcon className="size-4" />
              New folder
            </button>
            <button
              className="inline-flex h-11 items-center gap-2 rounded-xl bg-blue-700 px-4 text-sm font-semibold text-white transition hover:bg-blue-800 disabled:cursor-not-allowed disabled:opacity-50"
              disabled={storageFull}
              onClick={() => inputRef.current?.click()}
              title={storageFull ? 'Storage is full' : undefined}
              type="button"
            >
              <UploadIcon className="size-4" />
              Upload files
            </button>
          </div>
        </div>

        <h1 className="mt-6 text-2xl font-semibold tracking-[-0.03em] text-slate-950 sm:text-3xl">{currentName}</h1>

        {usage && (
          <div className="mt-4" data-testid="storage-usage">
            <div className="flex items-center justify-between text-xs font-medium text-slate-500">
              <span data-testid="storage-usage-text">
                {formatBytes(usage.used_bytes)} of {formatBytes(usage.limit_bytes)} used
                {storageFull && <span className="ml-2 font-semibold text-red-600">Storage full</span>}
              </span>
              <span>
                {usage.file_count} {usage.file_count === 1 ? 'file' : 'files'}
              </span>
            </div>
            <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-slate-200">
              <div
                className={`h-full rounded-full transition-all ${storageFull ? 'bg-red-500' : 'bg-blue-600'}`}
                style={{ width: `${usagePercent}%` }}
              />
            </div>
          </div>
        )}

        {error && (
          <div
            className="mt-4 flex items-start justify-between gap-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700"
            role="alert"
          >
            <p>{error}</p>
            <button
              aria-label="Dismiss error"
              className="font-semibold text-red-400 transition hover:text-red-700"
              onClick={() => setError('')}
              type="button"
            >
              Dismiss
            </button>
          </div>
        )}

        <div
          className={`mt-5 rounded-3xl border-2 border-dashed p-7 text-center transition sm:p-9 ${
            hover === 'upload'
              ? 'border-blue-500 bg-blue-50'
              : 'border-slate-300 bg-white hover:border-blue-400 hover:bg-blue-50/40'
          }`}
          onClick={() => {
            if (!storageFull) inputRef.current?.click();
          }}
          role="button"
          tabIndex={0}
          onKeyDown={(event) => {
            if ((event.key === 'Enter' || event.key === ' ') && !storageFull) inputRef.current?.click();
          }}
          {...uploadZone}
        >
          <span className="mx-auto grid size-12 place-items-center rounded-2xl bg-blue-50 text-blue-700">
            <UploadIcon className="size-6" />
          </span>
          <p className="mt-4 text-sm font-semibold text-slate-900">Drag and drop files here</p>
          {storageFull ? (
            <p className="mt-1 text-sm font-medium text-red-600">
              Storage is full — delete files to upload more.
            </p>
          ) : (
            <p className="mt-1 text-sm text-slate-500">
              or <span className="font-semibold text-blue-700">browse your device</span>
              {' — uploads go into '}
              <span className="font-semibold text-slate-700">{currentName}</span>
            </p>
          )}
          {uploading > 0 && (
            <p className="mt-3 inline-flex items-center gap-2 rounded-full bg-blue-700 px-3.5 py-1.5 text-xs font-semibold text-white">
              <Spinner className="size-3.5" />
              Uploading {uploading} {uploading === 1 ? 'file' : 'files'}…
            </p>
          )}
        </div>

        <input
          className="hidden"
          multiple
          onChange={(event) => {
            const chosen = event.target.files;
            if (chosen && chosen.length > 0) void handleUpload(chosen, folderId ?? null);
            event.target.value = '';
          }}
          ref={inputRef}
          type="file"
        />

        <div
          className={`mt-6 rounded-3xl border p-5 transition sm:p-6 ${
            hover === 'grid' ? 'border-blue-500 bg-blue-50/60' : 'border-slate-200 bg-white'
          }`}
          {...gridZone}
        >
          {loading ? (
            <div className="grid min-h-52 place-items-center text-slate-400">
              <Spinner className="size-7" />
            </div>
          ) : isEmpty ? (
            <div className="grid min-h-52 place-items-center px-4 text-center">
              <div>
                <span className="mx-auto grid size-12 place-items-center rounded-2xl bg-slate-100 text-slate-500">
                  <FolderIcon className="size-6" />
                </span>
                <p className="mt-4 text-sm font-semibold text-slate-900">This folder is empty</p>
                <p className="mt-1 text-sm text-slate-500">
                  Drop files here, create a folder, or drag an item in from somewhere else.
                </p>
              </div>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {folders.map((folder) => {
                const { zone, root } = folderTileProps(folder);
                const highlighted = hover === folder.id;
                return (
                  <div
                    className={`group relative rounded-2xl border p-4 transition ${
                      highlighted
                        ? 'border-blue-500 bg-blue-50 ring-2 ring-blue-500/40'
                        : 'border-slate-200 bg-white hover:border-slate-300 hover:shadow-sm'
                    }`}
                    key={folder.id}
                    {...root}
                    {...zone}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <span className="grid size-11 place-items-center rounded-xl bg-blue-50 text-blue-700">
                        <FolderIcon className="size-5.5" />
                      </span>
                      <KebabMenu
                        actions={[
                          {
                            label: 'Delete',
                            danger: true,
                            icon: <TrashIcon className="size-4" />,
                            onSelect: () =>
                              setDeleteTarget({ kind: 'folder', id: folder.id, name: folder.name }),
                          },
                        ]}
                        label={`Actions for folder ${folder.name}`}
                      />
                    </div>
                    <Link className="mt-3 block rounded-lg focus:outline-none focus-visible:ring-4 focus-visible:ring-blue-500/20" to={`/f/${folder.id}`}>
                      <p className="truncate text-sm font-semibold text-slate-900">{folder.name}</p>
                      <p className="mt-1 text-xs text-slate-500">Folder · {formatDate(folder.created_at)}</p>
                    </Link>
                  </div>
                );
              })}

              {files.map((file) => (
                <div
                  className="group relative rounded-2xl border border-slate-200 bg-white p-4 transition hover:border-slate-300 hover:shadow-sm"
                  key={file.id}
                  {...fileTileProps(file)}
                >
                  <div className="flex items-start justify-between gap-2">
                    <span className="grid size-11 place-items-center rounded-xl bg-violet-50 text-violet-700">
                      <FileIcon className="size-5.5" />
                    </span>
                    <KebabMenu
                      actions={[
                        {
                          label: 'Share',
                          icon: <ShareIcon className="size-4" />,
                          onSelect: () => setShareFile(file),
                        },
                        {
                          label: 'Delete',
                          danger: true,
                          icon: <TrashIcon className="size-4" />,
                          onSelect: () =>
                            setDeleteTarget({ kind: 'file', id: file.id, name: file.filename }),
                        },
                      ]}
                      label={`Actions for file ${file.filename}`}
                    />
                  </div>
                  <button
                    className="mt-3 block w-full rounded-lg text-left focus:outline-none focus-visible:ring-4 focus-visible:ring-blue-500/20"
                    onClick={() =>
                      void downloadFile(file).catch((cause: unknown) =>
                        setError(messageOf(cause, 'Download failed.')),
                      )
                    }
                    title={`Download ${file.filename}`}
                    type="button"
                  >
                    <span className="block truncate text-sm font-semibold text-slate-900">
                      {file.filename}
                    </span>
                    <span className="mt-1 flex items-center gap-1.5 text-xs text-slate-500">
                      {formatBytes(file.size_bytes)} · {formatDate(file.created_at)}
                      {file.status !== 'ready' && (
                        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-700">
                          {file.status}
                        </span>
                      )}
                    </span>
                  </button>
                  <span className="pointer-events-none absolute bottom-4 right-4 hidden text-slate-300 group-hover:block">
                    <DownloadIcon className="size-4" />
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </section>

      {newFolderOpen && (
        <NewFolderDialog
          defaultParentId={folderId ?? null}
          onClose={() => setNewFolderOpen(false)}
          onCreated={refresh}
        />
      )}
      {shareFile && <ShareDialog file={shareFile} onClose={() => setShareFile(null)} />}
      {sharesOpen && <SharesDialog onClose={() => setSharesOpen(false)} />}
      {deleteTarget && (
        <ConfirmDialog
          confirmLabel="Delete"
          message={
            deleteTarget.kind === 'file'
              ? `“${deleteTarget.name}” will be permanently deleted. This cannot be undone.`
              : `“${deleteTarget.name}” will be permanently deleted. The folder must be empty.`
          }
          onClose={() => setDeleteTarget(null)}
          onConfirm={confirmDelete}
          title={`Delete ${deleteTarget.kind}`}
        />
      )}
    </main>
  );
}
