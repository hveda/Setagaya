/**
 * The structured half of an action error (phase 24): when the backend's
 * error envelope carried details -- the 429 quota refusal's numbers and PUT
 * remediation, the 409 engines-finished conflict's purge hint -- render the
 * hint as a copyable <code> line and, for quota refusals, the
 * used/ceiling/requested arithmetic. Rendered only under an existing
 * message line; returns null when there are no details, so plain errors
 * look exactly as before.
 */
export default function ActionErrorDetails({ details }: { details: Record<string, unknown> | null }) {
  if (!details) {
    return null;
  }
  const hint = typeof details.hint === 'string' && details.hint !== '' ? details.hint : null;
  const num = (v: unknown): number | null => (typeof v === 'number' && Number.isFinite(v) ? v : null);
  const ceiling = num(details.ceiling);
  const used = num(details.used);
  const requested = num(details.requested);
  const show = (v: number | null): string => (v === null ? '?' : String(v));
  return (
    <div data-testid="action-error-details" className="mt-1 space-y-1">
      {hint !== null && (
        <p className="text-xs text-red-600 dark:text-red-400">
          <code className="rounded bg-red-100 px-1 py-0.5 font-mono text-xs text-red-800 dark:bg-red-900/40 dark:text-red-200">
            {hint}
          </code>
        </p>
      )}
      {ceiling !== null && (
        <p className="text-xs text-red-600 dark:text-red-400">
          used {show(used)} / ceiling {show(ceiling)} — requested {show(requested)}
        </p>
      )}
    </div>
  );
}
