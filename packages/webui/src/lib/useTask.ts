import { useRef, useState } from "react";
import {
  call,
  callTask,
  identity,
  read,
  type Run,
  type View,
  SavedStateRequired,
} from "./api";
import { SelectionRequests } from "./requests";

export function useTask() {
  const requests = useRef(new SelectionRequests());
  const writing = useRef(false);
  const [view, setView] = useState<View | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const outputRequest = useRef(0);
  const [error, setError] = useState("");
  const [mustRead, setMustRead] = useState(false);
  const [run, setRun] = useState<Run | null>(null);

  async function select(id: string, refresh = false) {
    const previous = requests.current.capture().id;
    const ticket = requests.current.select(id);
    if (previous !== id) setView(null);
    outputRequest.current++;
    setRun(null);
    setError("");
    setLoading(true);
    try {
      const response = await read<{ result: View }>(
        "/api/tasks/" + encodeURIComponent(id) + (refresh ? "?refresh=1" : ""),
      );
      if (requests.current.current(ticket, id)) {
        setView(response.result);
        setMustRead(false);
      }
    } catch (error) {
      if (requests.current.current(ticket, id)) setError(String(error));
    } finally {
      if (requests.current.current(ticket, id)) setLoading(false);
    }
  }
  function clear() {
    requests.current.select("");
    setView(null);
    outputRequest.current++;
    setRun(null);
    setLoading(false);
  }
  async function mutate(tool: string, fields: Record<string, unknown>) {
    if (!view || writing.current || mustRead) return;
    const selection = requests.current.capture();
    const args = { ...identity(view), ...fields };
    writing.current = true;
    setSaving(true);
    setError("");
    try {
      const next = await callTask(tool, args);
      if (requests.current.current(selection.sequence, selection.id)) {
        setView(next);
        outputRequest.current++;
        setRun(null);
      }
    } catch (error) {
      if (requests.current.current(selection.sequence, selection.id)) {
        setError(String(error));
        if (error instanceof SavedStateRequired) setMustRead(true);
      }
    } finally {
      writing.current = false;
      setSaving(false);
    }
  }
  async function readRun(id: string) {
    const output = ++outputRequest.current;
    const selection = requests.current.capture();
    try {
      const result = await call<Run>("taskbelay_get_run", { run_id: id });
      if (
        output === outputRequest.current &&
        requests.current.current(selection.sequence, selection.id)
      )
        setRun(result);
    } catch (error) {
      if (output === outputRequest.current && requests.current.current(selection.sequence, selection.id)) {
        setError(String(error));
        if (error instanceof SavedStateRequired) setMustRead(true);
      }
    }
  }
  return {
    view,
    busy: loading || saving,
    mutationBlocked: loading || saving || mustRead,
    error,
    run,
    select,
    clear,
    mutate,
    readRun,
  };
}
