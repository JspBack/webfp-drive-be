import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { register, signIn, startGitHubLogin } from '../lib/api';

type AuthMode = 'sign-in' | 'sign-up';

export function Auth() {
  const [mode, setMode] = useState<AuthMode>('sign-in');
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      if (mode === 'sign-up') {
        await register(name.trim(), email, password);
      } else {
        await signIn(email, password);
      }
      navigate('/', { replace: true });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to sign in.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="min-h-screen bg-white text-slate-900 lg:grid lg:grid-cols-2">
      <aside className="relative hidden min-h-screen overflow-hidden bg-slate-950 px-12 py-10 text-white lg:flex lg:flex-col xl:px-20">
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_18%_12%,rgba(59,130,246,0.35),transparent_38%),radial-gradient(ellipse_at_85%_80%,rgba(14,165,233,0.2),transparent_34%)]" />
        <div className="absolute inset-0 opacity-[0.14] [background-image:linear-gradient(rgba(255,255,255,.18)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,.18)_1px,transparent_1px)] [background-size:64px_64px] [mask-image:linear-gradient(to_bottom,black,transparent_78%)]" />
        <div className="relative flex h-full flex-col">
          <Brand light />
          <div className="my-auto max-w-xl py-20">
            <h2 className="max-w-lg text-5xl font-semibold leading-[1.08] tracking-[-0.045em] xl:text-6xl">
              Your files.
              <br />
              <span className="bg-linear-to-r from-blue-300 to-cyan-200 bg-clip-text text-transparent">
                Finally in order.
              </span>
            </h2>
            <p className="mt-6 max-w-md text-base leading-7 text-slate-300">
              A calmer place for the things you create, share, and want to keep close.
            </p>

            <div className="mt-12 rounded-2xl border border-white/10 bg-white/[0.07] p-5 shadow-2xl shadow-black/20 backdrop-blur-sm">
              <div className="flex items-center justify-between border-b border-white/10 pb-4">
                <div className="flex items-center gap-3">
                  <div className="grid size-10 place-items-center rounded-xl bg-blue-400/15 text-blue-200">
                    <FolderIcon />
                  </div>
                  <div>
                    <p className="text-sm font-medium text-white">Project files</p>
                    <p className="mt-0.5 text-xs text-slate-400">Updated just now</p>
                  </div>
                </div>
                <span className="rounded-full bg-emerald-400/10 px-2.5 py-1 text-xs font-medium text-emerald-300">
                  Synced
                </span>
              </div>
              <div className="mt-4 grid grid-cols-3 gap-3">
                <FileTile label="Design.fig" color="bg-violet-400/15 text-violet-200" />
                <FileTile label="Roadmap.pdf" color="bg-rose-400/15 text-rose-200" />
                <FileTile label="Assets.zip" color="bg-amber-400/15 text-amber-200" />
              </div>
            </div>
          </div>
        </div>
      </aside>

      <section className="flex min-h-screen flex-col px-6 py-7 sm:px-10 lg:px-12 xl:px-20">
        <div className="flex items-center justify-between">
          <div className="lg:hidden"><Brand /></div>
          <Link
            className="ml-auto text-sm font-medium text-slate-500 transition hover:text-slate-900"
            to="/"
          >
            Back to home
          </Link>
        </div>

        <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center py-12">
          <div className="mb-9">
            <p className="mb-3 text-sm font-semibold tracking-wide text-blue-700">
              {mode === 'sign-in' ? 'WELCOME BACK' : 'GET STARTED'}
            </p>
            <h1 className="text-3xl font-semibold tracking-[-0.04em] text-slate-950 sm:text-4xl">
              {mode === 'sign-in' ? 'Sign in to your drive' : 'Create your account'}
            </h1>
            <p className="mt-3 text-sm leading-6 text-slate-500">
              {mode === 'sign-in'
                ? 'Pick up right where you left off.'
                : 'Your secure space for all your files starts here.'}
            </p>
          </div>

          <button
            className="flex h-12 w-full items-center justify-center gap-3 rounded-xl border border-slate-200 bg-slate-950 text-sm font-semibold text-white shadow-sm transition hover:bg-slate-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900 disabled:cursor-wait disabled:opacity-60"
            disabled={busy}
            onClick={startGitHubLogin}
            type="button"
          >
            <GitHubMark />
            Continue with GitHub
          </button>

          <div className="my-7 flex items-center gap-4 text-xs font-medium text-slate-400">
            <span className="h-px flex-1 bg-slate-200" />
            OR CONTINUE WITH EMAIL
            <span className="h-px flex-1 bg-slate-200" />
          </div>

          <form className="space-y-5" onSubmit={submit}>
            {mode === 'sign-up' && (
              <label className="block">
                <span className="mb-2 block text-sm font-medium text-slate-700">Full name</span>
                <input
                  autoComplete="name"
                  className="h-12 w-full rounded-xl border border-slate-200 bg-white px-3.5 text-sm text-slate-900 outline-none transition placeholder:text-slate-400 hover:border-slate-300 focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Your name"
                  required
                  value={name}
                />
              </label>
            )}
            <label className="block">
              <span className="mb-2 block text-sm font-medium text-slate-700">Email address</span>
              <input
                autoComplete="email"
                className="h-12 w-full rounded-xl border border-slate-200 bg-white px-3.5 text-sm text-slate-900 outline-none transition placeholder:text-slate-400 hover:border-slate-300 focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@example.com"
                required
                type="email"
                value={email}
              />
            </label>
            <label className="block">
              <span className="mb-2 block text-sm font-medium text-slate-700">Password</span>
              <input
                autoComplete={mode === 'sign-in' ? 'current-password' : 'new-password'}
                className="h-12 w-full rounded-xl border border-slate-200 bg-white px-3.5 text-sm text-slate-900 outline-none transition placeholder:text-slate-400 hover:border-slate-300 focus:border-blue-500 focus:ring-4 focus:ring-blue-500/10"
                onChange={(event) => setPassword(event.target.value)}
                placeholder="At least 8 characters"
                minLength={8}
                required
                type="password"
                value={password}
              />
            </label>
            {error && (
              <p
                className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-5 text-red-700"
                role="alert"
              >
                {error}
              </p>
            )}
            <button
              className="flex h-12 w-full items-center justify-center gap-2 rounded-xl bg-blue-700 px-4 text-sm font-semibold text-white shadow-lg shadow-blue-900/15 transition hover:bg-blue-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-700 disabled:cursor-wait disabled:opacity-60"
              disabled={busy}
              type="submit"
            >
              {busy ? 'Please wait…' : mode === 'sign-in' ? 'Sign in' : 'Create account'}
              {!busy && <ArrowIcon />}
            </button>
          </form>

          <p className="mt-7 text-center text-sm text-slate-500">
            {mode === 'sign-in' ? 'New to WebFP Drive?' : 'Already have an account?'}{' '}
            <button
              className="font-semibold text-blue-700 transition hover:text-blue-900 disabled:opacity-60"
              disabled={busy}
              onClick={() => {
                setMode(mode === 'sign-in' ? 'sign-up' : 'sign-in');
                setError('');
              }}
              type="button"
            >
              {mode === 'sign-in' ? 'Create an account' : 'Sign in'}
            </button>
          </p>
        </div>

        <footer className="text-center text-xs text-slate-400">
          Your files stay yours. Always.
        </footer>
      </section>
    </main>
  );
}

function Brand({ light = false }: { light?: boolean }) {
  return (
    <Link className="inline-flex w-fit items-center gap-3" to="/">
      <span className={`grid size-10 place-items-center rounded-xl ${light ? 'bg-blue-500 text-white' : 'bg-blue-700 text-white'} shadow-lg shadow-blue-950/15`}>
        <DriveIcon />
      </span>
      <span className={`text-base font-semibold tracking-tight ${light ? 'text-white' : 'text-slate-900'}`}>
        WebFP Drive
      </span>
    </Link>
  );
}

function FileTile({ label, color }: { label: string; color: string }) {
  return (
    <div className="rounded-xl border border-white/10 bg-black/10 p-3">
      <div className={`mb-3 grid size-9 place-items-center rounded-lg ${color}`}>
        <DocumentIcon />
      </div>
      <p className="truncate text-[11px] font-medium text-slate-300">{label}</p>
    </div>
  );
}

function GitHubMark() {
  return (
    <svg aria-hidden="true" className="size-[18px]" fill="currentColor" viewBox="0 0 24 24">
      <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
    </svg>
  );
}

function DriveIcon() {
  return (
    <svg aria-hidden="true" className="size-5" fill="none" viewBox="0 0 24 24">
      <path d="m8.5 3.75-6 10.5 3.75 6.5h12l3.25-5.75-6.5-11.25h-6.5Z" stroke="currentColor" strokeLinejoin="round" strokeWidth="1.8" />
      <path d="m2.5 14.25 12.5-.5m-6.5-10 6.5 11.25" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" />
    </svg>
  );
}

function FolderIcon() {
  return (
    <svg aria-hidden="true" className="size-5" fill="none" viewBox="0 0 24 24">
      <path d="M3.5 7.75a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2v-10.5Z" stroke="currentColor" strokeLinejoin="round" strokeWidth="1.7" />
      <path d="M3.75 10h16.5" stroke="currentColor" strokeWidth="1.7" />
    </svg>
  );
}

function DocumentIcon() {
  return (
    <svg aria-hidden="true" className="size-5" fill="none" viewBox="0 0 24 24">
      <path d="M7 3.75h6.5l4.75 4.75v11.75H7a2 2 0 0 1-2-2V5.75a2 2 0 0 1 2-2Z" stroke="currentColor" strokeLinejoin="round" strokeWidth="1.6" />
      <path d="M13 4v5h5M8.5 13h6M8.5 16.5h6" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.6" />
    </svg>
  );
}

function ArrowIcon() {
  return (
    <svg aria-hidden="true" className="size-4" fill="none" viewBox="0 0 20 20">
      <path d="M4.167 10h11.666m0 0-5-5m5 5-5 5" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.7" />
    </svg>
  );
}
