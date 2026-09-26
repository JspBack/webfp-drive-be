type IconProps = {
  className?: string;
};

function base(className?: string): string {
  return className ?? 'size-5';
}

export function FolderIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M3.5 7.75a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2V7.75Z"
        stroke="currentColor"
        strokeLinejoin="round"
        strokeWidth="1.7"
      />
      <path d="M3.75 10h16.5" stroke="currentColor" strokeWidth="1.7" />
    </svg>
  );
}

export function FileIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5Z"
        stroke="currentColor"
        strokeLinejoin="round"
        strokeWidth="1.7"
      />
      <path d="M14 3v5h5" stroke="currentColor" strokeLinejoin="round" strokeWidth="1.7" />
      <path d="M8.5 13h7M8.5 16.5h4.5" stroke="currentColor" strokeLinecap="round" strokeWidth="1.7" />
    </svg>
  );
}

export function MoreIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="currentColor" viewBox="0 0 20 20">
      <circle cx="10" cy="4.25" r="1.6" />
      <circle cx="10" cy="10" r="1.6" />
      <circle cx="10" cy="15.75" r="1.6" />
    </svg>
  );
}

export function PlusIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path d="M12 5v14M5 12h14" stroke="currentColor" strokeLinecap="round" strokeWidth="1.9" />
    </svg>
  );
}

export function UploadIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M12 16V4m0 0L7.5 8.5M12 4l4.5 4.5"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
      <path
        d="M4.5 15v3a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-3"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

export function ChevronRightIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path d="m9 5.5 6.5 6.5L9 18.5" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" />
    </svg>
  );
}

export function TrashIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M4.5 6.75h15M9.75 6.75V5a1.25 1.25 0 0 1 1.25-1.25h2A1.25 1.25 0 0 1 14.25 5v1.75m3.5 0-.7 11.6a2 2 0 0 1-2 1.9h-5.6a2 2 0 0 1-2-1.9l-.7-11.6"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.7"
      />
      <path d="M10.25 10.5v5.5M13.75 10.5v5.5" stroke="currentColor" strokeLinecap="round" strokeWidth="1.7" />
    </svg>
  );
}

export function ShareIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M13.5 4.5 19.5 10.5M19.5 10.5 13.5 16.5M19.5 10.5H9.75a5.25 5.25 0 0 0-5.25 5.25V19"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.7"
      />
      <circle cx="18" cy="6" r="2.6" stroke="currentColor" strokeWidth="1.7" />
      <circle cx="6" cy="18.4" r="2.6" stroke="currentColor" strokeWidth="1.7" />
    </svg>
  );
}

export function DownloadIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M12 4v11m0 0-4.5-4.5M12 15l4.5-4.5M4.5 17.5v1a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-1"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

export function CopyIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <rect height="11" rx="2" stroke="currentColor" strokeWidth="1.7" width="11" x="8.5" y="8.5" />
      <path
        d="M15.5 8.5v-2a2 2 0 0 0-2-2h-7a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h2"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.7"
      />
    </svg>
  );
}

export function XIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path d="m6.5 6.5 11 11m0-11-11 11" stroke="currentColor" strokeLinecap="round" strokeWidth="1.9" />
    </svg>
  );
}

export function LinkIcon({ className }: IconProps) {
  return (
    <svg aria-hidden="true" className={base(className)} fill="none" viewBox="0 0 24 24">
      <path
        d="M10.5 13.5a3.75 3.75 0 0 0 5.36.3l2.14-2.14a3.75 3.75 0 0 0-5.3-5.3l-1.23 1.22M13.5 10.5a3.75 3.75 0 0 0-5.36-.3L6 12.34a3.75 3.75 0 0 0 5.3 5.3l1.22-1.22"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.7"
      />
    </svg>
  );
}

export function Spinner({ className }: IconProps) {
  return (
    <span
      aria-hidden="true"
      className={`inline-block animate-spin rounded-full border-2 border-current border-t-transparent ${className ?? 'size-4'}`}
    />
  );
}
