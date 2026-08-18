// Engine badge: the engine kind that produced a run or execution (jmeter,
// k6, ...) -- rendered only when the wire carries one. Extracted from
// Reports.tsx when LiveStatus needed it too.
export default function EngineBadge({ engine }: { engine: string }) {
  return (
    <span className="inline-flex items-center rounded-full bg-sky-100 px-2.5 py-0.5 text-xs font-medium text-sky-800 dark:bg-sky-900/30 dark:text-sky-300">
      {engine}
    </span>
  );
}
