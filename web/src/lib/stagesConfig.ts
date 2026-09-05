// The stage table's pure model: rows in the visual editor <-> the config
// JSON the backend's loadprofile.Profile/Entry decode. The JSON contract
// is newTestFlow.buildConfig's output -- the single-row case must be
// byte-equal to it -- and the field order mirrors Go's json marshal order
// (name, scenario_id, concurrency, rampup, engines, throughput, duration,
// csv_split), which is why throughput is inserted between engines and
// duration rather than appended.

/** One visual stage row. `throughput`/`csvSplit` omitted = their defaults. */
export interface StageRow {
  name: string;
  scenarioId: number;
  concurrency: number;
  rampup: number;
  engines: number;
  /** Target req/s shared across the row's engines. Omitted/0 = unlimited. */
  throughput?: number;
  duration: number;
  /** Split a CSV dataset across engines. Omitted/false = off. */
  csvSplit?: boolean;
}

/** One tests[] entry on the wire (loadprofile.Entry's JSON shape). */
export interface StageTestJSON {
  name: string;
  scenario_id: number;
  concurrency: number;
  rampup: number;
  engines: number;
  throughput?: number;
  duration: number;
  csv_split?: boolean;
}

/** The wrapper the flow PUTs to /executions/{id}/config (buildConfig's shape). */
export interface StagesConfigJSON {
  name: string;
  project_id: number;
  execution_id: number;
  tests: StageTestJSON[];
}

/**
 * Rows -> config JSON. Omission rules match the backend's omitempty
 * intent: throughput 0/undefined is unlimited and the key is omitted
 * (loadprofile.Throughput); csvSplit false is omitted. Note Go's Entry
 * tag for csv_split lacks omitempty so the backend may echo an explicit
 * false -- configToStages accepts both, we simply never write false.
 */
export function stagesToConfig(
  rows: StageRow[],
  name: string,
  projectId: number,
  executionId: number,
): StagesConfigJSON {
  return {
    name,
    project_id: projectId,
    execution_id: executionId,
    tests: rows.map((r): StageTestJSON => {
      const omitTp = r.throughput === undefined || r.throughput === 0;
      return {
        name: r.name,
        scenario_id: r.scenarioId,
        concurrency: r.concurrency,
        rampup: r.rampup,
        engines: r.engines,
        // Conditional spreads keep the key at this position whether or
        // not it is present -- Go marshal order, and byte-equality with
        // buildConfig when both optional keys are absent.
        ...(omitTp ? {} : { throughput: r.throughput }),
        duration: r.duration,
        ...(r.csvSplit ? { csv_split: true as const } : {}),
      };
    }),
  };
}

/**
 * Config JSON -> rows. Mechanical, never validating (see validateStageRow
 * for that); tolerant of absent keys and canonicalizes the omitted
 * defaults: throughput 0/absent -> undefined (unlimited), csv_split
 * false/absent -> undefined (off).
 */
export function configToStages(cfg: StagesConfigJSON): StageRow[] {
  const tests = Array.isArray(cfg?.tests) ? cfg.tests : [];
  return tests.map((t) => ({
    name: t.name,
    scenarioId: t.scenario_id,
    concurrency: t.concurrency,
    rampup: t.rampup,
    engines: t.engines,
    throughput: t.throughput === 0 || t.throughput === undefined ? undefined : t.throughput,
    duration: t.duration,
    csvSplit: t.csv_split ? true : undefined,
  }));
}
