import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { saveSession } from '../lib/api';

type Outcome = { signedIn: true } | { signedIn: false; message: string };

function readCallback(): Outcome {
  const params = new URLSearchParams(window.location.hash.slice(1));
  window.history.replaceState(null, '', window.location.pathname);

  const token = params.get('token');
  const refreshToken = params.get('refresh_token');
  if (token && refreshToken) {
    saveSession(token, refreshToken);
    return { signedIn: true };
  }
  return {
    signedIn: false,
    message: params.get('error') ?? 'GitHub sign-in did not return a session.',
  };
}

export function AuthCallback() {
  const [outcome] = useState(readCallback);
  const navigate = useNavigate();

  useEffect(() => {
    if (outcome.signedIn) navigate('/', { replace: true });
  }, [navigate, outcome]);

  const error = outcome.signedIn ? '' : outcome.message;

  return (
    <main className="grid min-h-screen place-items-center bg-slate-50 px-5 py-12 text-slate-900">
      <section
        className="w-full max-w-md rounded-3xl border border-slate-200 bg-white p-8 text-center shadow-xl shadow-slate-900/5 sm:p-10"
        aria-live="polite"
      >
        <div className="mx-auto mb-6 grid size-12 place-items-center rounded-2xl bg-slate-950 text-white shadow-lg shadow-slate-900/20">
          <svg aria-hidden="true" className="size-5" fill="currentColor" viewBox="0 0 24 24">
            <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
          </svg>
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-slate-950">
          {error ? 'Sign-in could not be completed' : 'Finishing sign-in…'}
        </h1>
        {!error && (
          <div className="mx-auto mt-5 size-5 animate-spin rounded-full border-2 border-slate-200 border-t-slate-900" />
        )}
        {error && (
          <>
            <p
              className="mt-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-left text-sm leading-5 text-red-700"
              role="alert"
            >
              {error}
            </p>
            <Link
              className="mt-6 inline-flex h-11 w-full items-center justify-center rounded-xl bg-slate-950 px-4 text-sm font-semibold text-white transition hover:bg-slate-800"
              to="/auth/sign-in"
            >
              Back to sign in
            </Link>
          </>
        )}
      </section>
    </main>
  );
}
