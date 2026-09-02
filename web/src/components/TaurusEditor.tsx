import { useEffect, useRef, useState } from 'react';
import { EditorView, basicSetup } from 'codemirror';
import { yaml } from '@codemirror/lang-yaml';
import Button from './ui/Button';
import { ApiError } from '../api/client';
import { getScenarioRequests, setScenarioRequests } from '../api/scenarios';

interface TaurusEditorProps {
  scenarioId: number;
}

/**
 * The Taurus fragment editor (R4 shell): a CodeMirror 6 surface over the
 * scenario's YAML fragment, loaded verbatim via G2 and saved verbatim via
 * G3. Byte-exactness is the contract -- the editor edits TEXT, and every
 * later feature (R5's form lens, R6's validation) reads and patches the
 * same text rather than a reserialized model.
 */
export default function TaurusEditor({ scenarioId }: TaurusEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // One EditorView per mount; re-pointed at each scenario via dispatch.
  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError(null);
    setSavedAt(null);
    setDirty(false);
    getScenarioRequests(scenarioId)
      .then((doc) => {
        if (!alive || !hostRef.current) {
          return;
        }
        viewRef.current?.destroy();
        const view = new EditorView({
          doc,
          extensions: [basicSetup, yaml(), EditorView.lineWrapping],
          parent: hostRef.current,
          dispatch: (tr) => {
            const v = viewRef.current;
            if (v) {
              v.update([tr]);
              if (tr.docChanged) {
                setDirty(true);
              }
            }
          },
        });
        viewRef.current = view;
      })
      .catch((err: unknown) => {
        if (alive) setError(err instanceof ApiError ? err.message : 'Failed to load fragment.');
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
      viewRef.current?.destroy();
      viewRef.current = null;
    };
  }, [scenarioId]);

  const save = () => {
    const view = viewRef.current;
    if (!view || saving) {
      return;
    }
    setSaving(true);
    setError(null);
    // The doc's string form is what goes on the wire -- no normalization,
    // no pretty-print. The backend validates and stores byte-for-byte.
    setScenarioRequests(scenarioId, view.state.doc.toString())
      .then(() => {
        setDirty(false);
        setSavedAt(new Date().toLocaleTimeString());
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Save failed.');
      })
      .finally(() => setSaving(false));
  };

  const reload = () => {
    setDirty(false);
    // Re-point the effect by keying off a state nudge: simplest honest
    // reload is re-running the load effect via scenarioId change -- but
    // since it hasn't changed, fetch and swap the doc directly.
    getScenarioRequests(scenarioId)
      .then((doc) => {
        const view = viewRef.current;
        if (view) {
          view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: doc } });
        }
        setError(null);
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Reload failed.');
      });
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={save} disabled={!dirty || saving || loading}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
        <Button variant="outline" onClick={reload} disabled={loading}>
          Reload
        </Button>
        <span className="text-caption text-slate-500 dark:text-slate-400">
          {loading ? 'Loading…' : savedAt ? `Saved at ${savedAt}` : dirty ? 'Unsaved changes' : 'Up to date'}
        </span>
      </div>
      {error && (
        <p className="text-sm text-red-600 dark:text-red-400" role="alert">
          {error}
        </p>
      )}
      <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700" ref={hostRef} />
    </div>
  );
}
