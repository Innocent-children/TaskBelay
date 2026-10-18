import { useEffect, useState } from "react";
import { call, type Run } from "./api";

export function useRun(id: string) {
  const [run, setRun] = useState<Run | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [loading, setLoading] = useState(true);
  const [revision, reload] = useState(0);
  useEffect(() => {
    let active = true;
    setRun(null);
    setError(null);
    setLoading(true);
    void call<Run>("taskbelay_get_run", { run_id: id })
      .then((result) => {
        if (active) setRun(result);
      })
      .catch((value) => {
        if (active)
          setError(value instanceof Error ? value : new Error(String(value)));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [id, revision]);
  return { run, error, loading, reload: () => reload((value) => value + 1) };
}
