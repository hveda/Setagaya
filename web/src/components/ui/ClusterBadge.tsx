// Cluster badge: the load origin. Absent cluster means the deployment
// default -- rendered as nothing, not as "default", so legacy rows stay
// quiet. Extracted from Reports.tsx when LiveStatus needed it too.
export default function ClusterBadge({ cluster }: { cluster: string }) {
  return (
    <span className="inline-flex items-center rounded-full bg-violet-100 px-2.5 py-0.5 text-xs font-medium text-violet-800 dark:bg-violet-900/30 dark:text-violet-300">
      {cluster}
    </span>
  );
}
