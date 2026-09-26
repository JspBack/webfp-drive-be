import { useEffect, useState, type FormEvent, type ReactNode } from 'react';

import type { DriveFile, ShareLink, ShareListItem } from '../lib/drive';
import { createShare, createFolder, listShares, revokeShare } from '../lib/drive';
import { formatDate } from '../lib/format';
import { CopyIcon, LinkIcon, PlusIcon, ShareIcon, Spinner, TrashIcon, XIcon } from './icons';

function messageOf(cause: unknown, fallback: string): string {
  return cause instanceof Error && cause.message ? cause.message : fallback;
}

export function Modal({
  title,
  description,
  onClose,
  children,
  footer,
}: {
  title: string;
  description?: string;
  onClose: () => void;
  children?: ReactNode;
  footer?: ReactNode;
}) {
  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center p-4"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
      role="presentation"
    >
      <div className="absolute inset-0 bg-slate-950/45 backdrop-blur-sm" />
      <div
        aria-labelledby="modal-title"
        className="relative w-full max-w-md rounded-3xl bg-white p-6 shadow-2xl shadow-slate-900/20"
        role="dialog"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold tracking-tight text-slate-950" id="modal-title">
              {title}
            </h2>
            {description && <p className="mt-1 text-sm leading-6 text-slate-500">{description}</p>}
          </div>
          <button
            aria-label="Close"
            className="-mr-1.5 -mt-1.5 grid size-9 place-items-center rounded-xl text-slate-400 transition hover:bg-slate-100 hover:text-slate-700"
            onClick={onClose}
            type="button"
          >
            <XIcon className="size-5" />
          </button>
        </div>
        {children}
        {footer && <div className="mt-6 flex justify-end gap-3">{footer}</div>}
      </div>
    </div>
  );
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  onConfirm,
  onClose,
}: {
  title: string;
  message: string;
  confirmLabel: string;
  onConfirm: () => Promise<void>;
  onClose: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function confirm() {
    setBusy(true);
    setError('');
    try {
      await onConfirm();
    } catch (cause) {
      setError(messageOf(cause, 'Something went wrong.'));
      setBusy(false);
      return;
    }
    setBusy(false);
    onClose();
  }

  return (
    <Modal
      description={message}
      footer={
        <>
          <button
            className="inline-flex h-11 items-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:bg-slate-50"
            disabled={busy}
            onClick={onClose}
            type="button"
          >
            Cancel
          </button>
          <button
            className="inline-flex h-11 items-center gap-2 rounded-xl bg-red-600 px-4 text-sm font-semibold text-white transition hover:bg-red-700 disabled:opacity-60"
            disabled={busy}
            onClick={() => void confirm()}
            type="button"
          >
            {busy ? <Spinner /> : <TrashIcon className="size-4" />}
            {confirmLabel}
          </button>
        </>
      }
      onClose={onClose}
      title={title}
    >
      {error && (
        <p className="mt-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700" role="alert">
          {error}
        </p>
      )}
    </Modal>
  );
}

export function NewFolderDialog({
  defaultParentId,
  onCreated,
  onClose,
}: {
  defaultParentId: string | null;
  onCreated: () => void;
  onClose: () => void;
}) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) {
      setError('Folder name is required.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await createFolder(trimmed, defaultParentId);
    } catch (cause) {
      setError(messageOf(cause, 'Unable to create the folder.'));
      setBusy(false);
      return;
    }
    setBusy(false);
    onCreated();
    onClose();
  }

  return (
    <Modal
      footer={
        <>
          <button
            className="inline-flex h-11 items-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:bg-slate-50"
            onClick={onClose}
            type="button"
          >
            Cancel
          </button>
          <button
            className="inline-flex h-11 items-center gap-2 rounded-xl bg-blue-700 px-4 text-sm font-semibold text-white transition hover:bg-blue-800 disabled:opacity-60"
            disabled={busy}
            form="new-folder-form"
            type="submit"
          >
            {busy ? <Spinner /> : <PlusIcon className="size-4" />}
            Create
          </button>
        </>
      }
      onClose={onClose}
      title="New folder"
    >
      <form className="mt-5" id="new-folder-form" onSubmit={(event) => void submit(event)}>
        <label className="block text-sm font-medium text-slate-700" htmlFor="folder-name">
          Folder name
        </label>
        <input
          autoFocus
          className="mt-2 h-12 w-full rounded-xl border border-slate-200 bg-white px-4 text-sm text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
          id="folder-name"
          onChange={(event) => setName(event.target.value)}
          placeholder="Design assets"
          value={name}
        />
        {error && (
          <p className="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700" role="alert">
            {error}
          </p>
        )}
      </form>
    </Modal>
  );
}

const EXPIRY_CHOICES = [
  { value: '1h', label: 'In 1 hour', hours: 1 },
  { value: '24h', label: 'In 24 hours', hours: 24 },
  { value: '7d', label: 'In 7 days', hours: 24 * 7 },
  { value: '30d', label: 'In 30 days', hours: 24 * 30 },
] as const;

export function ShareDialog({ file, onClose }: { file: DriveFile; onClose: () => void }) {
  const [expiry, setExpiry] = useState<string>('24h');
  const [maxDownloads, setMaxDownloads] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [share, setShare] = useState<ShareLink | null>(null);
  const [copied, setCopied] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const choice = EXPIRY_CHOICES.find((item) => item.value === expiry) ?? EXPIRY_CHOICES[1];
    const trimmedMax = maxDownloads.trim();
    let max: number | undefined;
    if (trimmedMax) {
      const parsed = Number(trimmedMax);
      if (!Number.isInteger(parsed) || parsed <= 0) {
        setError('Maximum downloads must be a positive whole number.');
        return;
      }
      max = parsed;
    }
    setBusy(true);
    setError('');
    try {
      const created = await createShare(file.id, {
        expiresAt: new Date(Date.now() + choice.hours * 3_600_000).toISOString(),
        maxDownloads: max,
        password: password || undefined,
      });
      setShare(created);
    } catch (cause) {
      setError(messageOf(cause, 'Unable to create the share link.'));
    }
    setBusy(false);
  }

  async function copyLink() {
    if (!share) return;
    try {
      await navigator.clipboard.writeText(share.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2_000);
    } catch {
      setError('Copy failed — select the link and copy it manually.');
    }
  }

  if (share) {
    return (
      <Modal onClose={onClose} title="Share link created">
        <div className="mt-5 rounded-2xl border border-emerald-200 bg-emerald-50 p-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-emerald-800">
            <LinkIcon className="size-4" />
            Anyone with the link can download “{file.filename}”
          </div>
          <p className="mt-2 text-xs text-emerald-700">
            Expires {new Date(share.expires_at).toLocaleString()}
          </p>
        </div>
        <label className="mt-4 block text-sm font-medium text-slate-700" htmlFor="share-url">
          Link
        </label>
        <div className="mt-2 flex gap-2">
          <input
            className="h-11 min-w-0 flex-1 rounded-xl border border-slate-200 bg-slate-50 px-3 text-sm text-slate-700 outline-none"
            id="share-url"
            onFocus={(event) => event.currentTarget.select()}
            readOnly
            value={share.url}
          />
          <button
            className="inline-flex h-11 items-center gap-2 rounded-xl bg-blue-700 px-4 text-sm font-semibold text-white transition hover:bg-blue-800"
            onClick={() => void copyLink()}
            type="button"
          >
            {copied ? 'Copied' : <CopyIcon className="size-4" />}
          </button>
        </div>
        {error && (
          <p className="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700" role="alert">
            {error}
          </p>
        )}
        <div className="mt-6 flex justify-end">
          <button
            className="inline-flex h-11 items-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:bg-slate-50"
            onClick={onClose}
            type="button"
          >
            Done
          </button>
        </div>
      </Modal>
    );
  }

  return (
    <Modal description={`Create a link that lets anyone download “${file.filename}”.`} onClose={onClose} title="Share file">
      <form className="mt-5 space-y-4" onSubmit={(event) => void submit(event)}>
        <div>
          <label className="block text-sm font-medium text-slate-700" htmlFor="share-expiry">
            Link expires
          </label>
          <select
            className="mt-2 h-12 w-full rounded-xl border border-slate-200 bg-white px-4 text-sm text-slate-900 outline-none transition focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
            id="share-expiry"
            onChange={(event) => setExpiry(event.target.value)}
            value={expiry}
          >
            {EXPIRY_CHOICES.map((choice) => (
              <option key={choice.value} value={choice.value}>
                {choice.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-sm font-medium text-slate-700" htmlFor="share-max">
            Maximum downloads <span className="font-normal text-slate-400">(optional)</span>
          </label>
          <input
            className="mt-2 h-12 w-full rounded-xl border border-slate-200 bg-white px-4 text-sm text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
            id="share-max"
            min="1"
            onChange={(event) => setMaxDownloads(event.target.value)}
            placeholder="Unlimited"
            type="number"
            value={maxDownloads}
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-slate-700" htmlFor="share-password">
            Password <span className="font-normal text-slate-400">(optional)</span>
          </label>
          <input
            className="mt-2 h-12 w-full rounded-xl border border-slate-200 bg-white px-4 text-sm text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
            id="share-password"
            onChange={(event) => setPassword(event.target.value)}
            placeholder="Require a password to download"
            type="password"
            value={password}
          />
        </div>
        {error && (
          <p className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700" role="alert">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3 pt-1">
          <button
            className="inline-flex h-11 items-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:bg-slate-50"
            onClick={onClose}
            type="button"
          >
            Cancel
          </button>
          <button
            className="inline-flex h-11 items-center gap-2 rounded-xl bg-blue-700 px-4 text-sm font-semibold text-white transition hover:bg-blue-800 disabled:opacity-60"
            disabled={busy}
            type="submit"
          >
            {busy ? <Spinner /> : <ShareIcon className="size-4" />}
            Create link
          </button>
        </div>
      </form>
    </Modal>
  );
}

export function SharesDialog({ onClose }: { onClose: () => void }) {
  const [shares, setShares] = useState<ShareListItem[] | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    void listShares()
      .then((items) => {
        if (cancelled) return;
        setShares(items);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setShares([]);
        setError(messageOf(cause, 'Unable to load your share links.'));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function copy(share: ShareListItem) {
    try {
      await navigator.clipboard.writeText(share.url);
      setCopiedId(share.id);
      setTimeout(() => setCopiedId((id) => (id === share.id ? null : id)), 2_000);
    } catch {
      setError('Copy failed — select the link and copy it manually.');
    }
  }

  async function revoke(share: ShareListItem) {
    setBusyId(share.id);
    setError('');
    try {
      await revokeShare(share.id);
      setShares((items) => (items ?? []).filter((item) => item.id !== share.id));
    } catch (cause) {
      setError(messageOf(cause, 'Unable to revoke that share link.'));
    } finally {
      setBusyId(null);
    }
  }

  return (
    <Modal
      description="Active links you have created. Revoking a link disables it immediately."
      onClose={onClose}
      title="Share links"
      footer={
        <button
          className="inline-flex h-11 items-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-600 transition hover:bg-slate-50"
          onClick={onClose}
          type="button"
        >
          Close
        </button>
      }
    >
      {error && (
        <p className="mt-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700" role="alert">
          {error}
        </p>
      )}
      <div className="mt-5 max-h-[55vh] overflow-y-auto pr-1">
        {shares === null ? (
          <div className="grid min-h-32 place-items-center text-slate-400">
            <Spinner className="size-6" />
          </div>
        ) : shares.length === 0 ? (
          <div className="grid min-h-32 place-items-center px-4 text-center">
            <div>
              <span className="mx-auto grid size-11 place-items-center rounded-2xl bg-slate-100 text-slate-500">
                <LinkIcon className="size-5" />
              </span>
              <p className="mt-3 text-sm font-semibold text-slate-900">No active share links</p>
              <p className="mt-1 text-xs leading-5 text-slate-500">
                Create one from a file's actions menu.
              </p>
            </div>
          </div>
        ) : (
          <ul className="space-y-3" data-testid="share-list">
            {shares.map((share) => (
              <li className="rounded-2xl border border-slate-200 p-3.5" data-testid="share-row" key={share.id}>
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold text-slate-900">{share.filename}</p>
                    <p className="mt-1 text-xs text-slate-500">
                      Expires {formatDate(share.expires_at)} · {share.download_count}{' '}
                      {share.download_count === 1 ? 'download' : 'downloads'}
                      {share.max_downloads > 0 ? ` of ${share.max_downloads}` : ''}
                      {share.has_password ? ' · password protected' : ''}
                    </p>
                  </div>
                  <span className="shrink-0 rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-500">
                    Active
                  </span>
                </div>
                <div className="mt-3 flex gap-2">
                  <input
                    aria-label={`Share link for ${share.filename}`}
                    className="h-10 min-w-0 flex-1 rounded-xl border border-slate-200 bg-slate-50 px-3 text-xs text-slate-700 outline-none"
                    onFocus={(event) => event.currentTarget.select()}
                    readOnly
                    value={share.url}
                  />
                  <button
                    className="inline-flex h-10 items-center gap-1.5 rounded-xl border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
                    onClick={() => void copy(share)}
                    type="button"
                  >
                    <CopyIcon className="size-3.5" />
                    {copiedId === share.id ? 'Copied' : 'Copy'}
                  </button>
                  <button
                    className="inline-flex h-10 items-center gap-1.5 rounded-xl border border-red-200 bg-white px-3 text-xs font-semibold text-red-600 transition hover:bg-red-50 disabled:opacity-60"
                    disabled={busyId === share.id}
                    onClick={() => void revoke(share)}
                    type="button"
                  >
                    {busyId === share.id ? <Spinner className="size-3.5" /> : <TrashIcon className="size-3.5" />}
                    Revoke
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
