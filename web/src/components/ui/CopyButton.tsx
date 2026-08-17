// Copy affordance for raw values (correlation ids): the whole point is that
// what lands on the clipboard is the exact raw id, never a formatted or
// truncated version. Uses the async Clipboard API when available and falls
// back to a select+execCommand textarea for embedded/insecure contexts
// (e.g. the SPA served over plain HTTP inside a cluster).
import { useEffect, useReducer, useRef } from 'react';
import { Check, Copy, TriangleAlert } from 'lucide-react';

export type CopyStatus = 'idle' | 'copied' | 'failed';
export type CopyAction = { type: 'copy' } | { type: 'succeeded' } | { type: 'failed' } | { type: 'reset' };

/** Transitions of the copy button's confirmed/failed state; 'copy' resets to idle for the in-flight moment. */
export function copyReducer(_state: CopyStatus, action: CopyAction): CopyStatus {
  switch (action.type) {
    case 'copy':
      return 'idle';
    case 'succeeded':
      return 'copied';
    case 'failed':
      return 'failed';
    case 'reset':
      return 'idle';
  }
}

/** Copies text via the Clipboard API, falling back to a hidden textarea + execCommand. Resolves true on success. */
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or insecure context; try the fallback.
    }
  }
  return copyViaTextarea(text);
}

function copyViaTextarea(text: string): boolean {
  const area = document.createElement('textarea');
  area.value = text;
  // Off-screen but focused: iOS ignores select() on display:none elements.
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.appendChild(area);
  area.focus();
  area.select();
  let ok = false;
  try {
    ok = document.execCommand('copy');
  } catch {
    ok = false;
  }
  document.body.removeChild(area);
  return ok;
}

export interface CopyButtonProps {
  /** The exact value to place on the clipboard. */
  value: string;
  /** Accessible label naming what is being copied (e.g. "Copy correlation id"). */
  label: string;
}

export default function CopyButton({ value, label }: CopyButtonProps) {
  const [status, dispatch] = useReducer(copyReducer, 'idle');
  const resetTimer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(resetTimer.current), []);

  const onCopy = async () => {
    dispatch({ type: 'copy' });
    dispatch((await copyText(value)) ? { type: 'succeeded' } : { type: 'failed' });
    window.clearTimeout(resetTimer.current);
    resetTimer.current = window.setTimeout(() => dispatch({ type: 'reset' }), 2000);
  };

  return (
    <button
      type="button"
      onClick={() => void onCopy()}
      aria-label={label}
      title={label}
      className="inline-flex min-h-[32px] items-center gap-1 rounded-md border border-slate-300 px-2 py-1 text-caption text-slate-600 transition-colors hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-sky-500 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800"
    >
      {status === 'copied' ? (
        <Check aria-hidden className="h-3.5 w-3.5" />
      ) : status === 'failed' ? (
        <TriangleAlert aria-hidden className="h-3.5 w-3.5" />
      ) : (
        <Copy aria-hidden className="h-3.5 w-3.5" />
      )}
      <span aria-live="polite">{status === 'copied' ? 'Copied' : status === 'failed' ? 'Copy failed' : 'Copy'}</span>
    </button>
  );
}
