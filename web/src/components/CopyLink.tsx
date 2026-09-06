// "Copy link" affordance for deep-linkable pages (Reports run detail,
// Execution): puts the page's current URL on the clipboard so an operator
// can hand someone else the exact view they are looking at. The clipboard
// mechanics are CopyButton's copyText -- the async Clipboard API when
// available, the select+execCommand textarea fallback for embedded or
// insecure contexts -- reused rather than reimplemented; this component
// adds only the page-URL sourcing and the share-button presentation.
import { useEffect, useRef, useState } from 'react';
import { Check, Link2, TriangleAlert } from 'lucide-react';
import { copyText } from './ui/CopyButton';

export default function CopyLink() {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  const resetTimer = useRef<number | undefined>(undefined);

  // The confirmation is fleeting by design; if the page unmounts first the
  // pending reset must not fire against a gone component.
  useEffect(() => () => window.clearTimeout(resetTimer.current), []);

  const onCopy = async () => {
    // Read at click time, not render time: the router can move this page
    // while it stays mounted, and the link copied must be the one shown.
    const ok = await copyText(window.location.href);
    setCopied(ok);
    setFailed(!ok);
    window.clearTimeout(resetTimer.current);
    resetTimer.current = window.setTimeout(() => {
      setCopied(false);
      setFailed(false);
    }, 2000);
  };

  return (
    <button
      type="button"
      data-testid="copy-link"
      onClick={() => void onCopy()}
      aria-label="Copy link to this page"
      title="Copy link to this page"
      className="inline-flex min-h-[32px] items-center gap-1 rounded-md border border-slate-300 px-2 py-1 text-caption font-medium text-slate-600 transition-colors hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-sky-500 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800"
    >
      {copied ? (
        <Check aria-hidden className="h-3.5 w-3.5" />
      ) : failed ? (
        <TriangleAlert aria-hidden className="h-3.5 w-3.5" />
      ) : (
        <Link2 aria-hidden className="h-3.5 w-3.5" />
      )}
      <span aria-live="polite">{copied ? 'Copied' : failed ? 'Copy failed' : 'Copy link'}</span>
    </button>
  );
}
