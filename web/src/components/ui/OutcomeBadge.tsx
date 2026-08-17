// Shared outcome badge (passed/failed/aborted/error): used by the Reports
// run rows/detail and the trend table. Extracted from Reports.tsx so
// sibling components can reuse it without a circular page import.
import type { Outcome } from '../../api/reports';

export const outcomeClasses: Record<Outcome, string> = {
  passed: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
  failed: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
  aborted: 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-300',
  error: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
};

export default function OutcomeBadge({ outcome }: { outcome: Outcome }) {
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${outcomeClasses[outcome]}`}>
      {outcome}
    </span>
  );
}
