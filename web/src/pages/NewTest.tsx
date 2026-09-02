import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Button from '../components/ui/Button';
import Card, { CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import { apiClient, ApiError } from '../api/client';
import { setScenarioRequests } from '../api/scenarios';
import {
  buildConfig,
  buildFragment,
  concurrencyEnginesWarning,
  stepError,
  flowSteps,
  type NewTestForm,
} from '../lib/newTestFlow';

interface ProjectRef {
  id: number;
  name: string;
}

const inputCls =
  'rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm text-slate-900 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100';

/**
 * R9: one form, zero identifiers. The flow creates everything in order
 * (project-if-absent -> scenario -> execution -> fragment -> config) and
 * navigates to the execution hub. A failure names the STEP that failed.
 */
export default function NewTest() {
  const navigate = useNavigate();
  const [form, setForm] = useState<NewTestForm>({
    name: '',
    targetUrl: '',
    method: 'GET',
    headers: [],
    concurrency: 50,
    engines: 2,
    // R9: ramp-up defaults NON-ZERO -- starting at full concurrency
    // measures connection-pool cold start, not steady state.
    rampup: 30,
    duration: 300,
    engine: 'jmeter',
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [step, setStep] = useState<string | null>(null);

  const warning = concurrencyEnginesWarning(form.concurrency, form.engines);
  const set = (patch: Partial<NewTestForm>) => setForm((f) => ({ ...f, ...patch }));
  const setHeader = (i: number, hpatch: Partial<{ name: string; value: string }>) =>
    set({ headers: form.headers.map((h, j) => (j === i ? { ...h, ...hpatch } : h)) });

  const submit = () => {
    if (!form.name.trim() || !form.targetUrl.trim()) {
      setError('Name and target URL are required.');
      return;
    }
    setBusy(true);
    setError(null);

    const run = async () => {
      // Step 1: resolve project-if-absent. The operator names a project;
      // the flow looks it up and creates it only when missing.
      setStep(flowSteps[0]);
      let project: ProjectRef;
      const projects = await apiClient.get<ProjectRef[] | null>('/projects').catch((e: unknown) => {
        throw stepError('resolve project', e);
      });
      const wanted = `tests-${form.name.trim().toLowerCase().replace(/\s+/g, '-')}`;
      const existing = (projects ?? []).find((p) => p.name === wanted);
      if (existing) {
        project = existing;
      } else {
        project = await apiClient
          .post<ProjectRef>('/projects', new URLSearchParams({ name: wanted, owner: 'honryu' }))
          .catch((e: unknown) => {
            throw stepError('resolve project', e);
          });
      }

      // Step 2: scenario (portable; no kind needed -- default).
      setStep(flowSteps[1]);
      const scenario = (await apiClient
        .post<{ id: number }>('/scenarios', new URLSearchParams({ project_id: String(project.id), name: form.name }))
        .catch((e: unknown) => {
          throw stepError('create scenario', e);
        })) as { id: number; scenario?: { id: number } };
      const scenarioId = scenario.id ?? scenario.scenario?.id;

      // Step 3: execution.
      setStep(flowSteps[2]);
      const execution = await apiClient
        .post<{ id: number }>(
          '/executions',
          new URLSearchParams({ project_id: String(project.id), name: form.name, engine: form.engine }),
        )
        .catch((e: unknown) => {
          throw stepError('create execution', e);
        });

      // Step 4: the requests fragment (G3, text/yaml verbatim).
      setStep(flowSteps[3]);
      await setScenarioRequests(scenarioId, buildFragment(form)).catch((e: unknown) => {
        throw stepError('save requests fragment', e);
      });

      // Step 5: the load config (G7's JSON body).
      setStep(flowSteps[4]);
      const cfg = buildConfig(form, scenarioId);
      cfg.project_id = project.id;
      cfg.execution_id = execution.id;
      await apiClient
        .putRaw(`/executions/${execution.id}/config`, 'application/json', JSON.stringify(cfg))
        .catch((e: unknown) => {
          throw stepError('save load config', e);
        });

      // Done: land on the deep-linkable hub.
      navigate(`/executions/${execution.id}`);
    };

    run()
      .catch((e: unknown) => {
        setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : 'Something failed.');
      })
      .finally(() => setBusy(false));
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-display-sm text-slate-900 dark:text-white">New test</h1>
        <p className="text-body-sm mt-1 text-slate-500 dark:text-slate-400">
          Describe the test; the project, scenario, and execution are created for you.
        </p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Test definition</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className="text-caption text-slate-600 dark:text-slate-300">
              Test name
              <input className={`${inputCls} mt-1 w-full`} value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="checkout-smoke" />
            </label>
            <label className="text-caption text-slate-600 dark:text-slate-300">
              Engine
              <select className={`${inputCls} mt-1 w-full`} value={form.engine} onChange={(e) => set({ engine: e.target.value })}>
                <option value="jmeter">jmeter</option>
                <option value="gatling">gatling</option>
              </select>
            </label>
          </div>
          <label className="block text-caption text-slate-600 dark:text-slate-300">
            Target URL
            <input className={`${inputCls} mt-1 w-full`} value={form.targetUrl} onChange={(e) => set({ targetUrl: e.target.value })} placeholder="http://checkout.svc" />
          </label>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-4">
            <label className="text-caption text-slate-600 dark:text-slate-300">
              Concurrency
              <input className={`${inputCls} mt-1 w-full`} type="number" min={1} value={form.concurrency} onChange={(e) => set({ concurrency: Number(e.target.value) })} />
            </label>
            <label className="text-caption text-slate-600 dark:text-slate-300">
              Engines
              <input className={`${inputCls} mt-1 w-full`} type="number" min={1} value={form.engines} onChange={(e) => set({ engines: Number(e.target.value) })} />
            </label>
            <label className="text-caption text-slate-600 dark:text-slate-300">
              Ramp-up (s)
              <input className={`${inputCls} mt-1 w-full`} type="number" min={0} value={form.rampup} onChange={(e) => set({ rampup: Number(e.target.value) })} />
            </label>
            <label className="text-caption text-slate-600 dark:text-slate-300">
              Duration (s)
              <input className={`${inputCls} mt-1 w-full`} type="number" min={1} value={form.duration} onChange={(e) => set({ duration: Number(e.target.value) })} />
            </label>
          </div>
          {warning && (
            <p className="rounded-md bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/30 dark:text-amber-200" role="alert">
              {warning}
            </p>
          )}
          <div>
            <div className="flex items-center justify-between">
              <p className="text-caption text-slate-600 dark:text-slate-300">Headers (a cookie is a header)</p>
              <Button variant="ghost" onClick={() => set({ headers: [...form.headers, { name: '', value: '' }] })}>
                + Add header
              </Button>
            </div>
            {form.headers.length === 0 && <p className="text-caption text-slate-400">No headers.</p>}
            <div className="mt-2 space-y-2">
              {form.headers.map((h, i) => (
                <div key={i} className="flex gap-2">
                  <input className={`${inputCls} flex-1`} value={h.name} onChange={(e) => setHeader(i, { name: e.target.value })} placeholder="X-Auth" aria-label="header name" />
                  <input className={`${inputCls} flex-1`} value={h.value} onChange={(e) => setHeader(i, { value: e.target.value })} placeholder="token" aria-label="header value" />
                  <Button variant="ghost" onClick={() => set({ headers: form.headers.filter((_, j) => j !== i) })}>
                    ✕
                  </Button>
                </div>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Button onClick={submit} disabled={busy}>
              {busy ? `Working — ${step ?? '…'}` : 'Create test'}
            </Button>
            {busy && step && <span className="text-caption text-slate-500 dark:text-slate-400">step: {step}</span>}
          </div>
          {error && (
            <p className="text-sm text-red-600 dark:text-red-400" role="alert">
              {error}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
