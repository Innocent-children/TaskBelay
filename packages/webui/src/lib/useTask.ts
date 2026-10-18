import { useRef, useState } from "react";
import {
  callTask,
  identity,
  read,
  SavedStateRequired,
  type MutationResult,
  type Summary,
  type TaskRecord,
  type View,
} from "./api";
import { sameTaskVersion, SelectionRequests } from "./requests";

export function useTask() {
  const requests = useRef(new SelectionRequests());
  const writing = useRef(false);
  const readSequence = useRef(0);
  const [selectedId, setSelectedId] = useState("");
  const [view, setView] = useState<View | null>(null);
  const currentView = useRef(view);
  currentView.current = view;
  const [summary, setSummary] = useState<Summary | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [mustRead, setMustRead] = useState(false);
  const readRequired = useRef(false);
  const [readAt, setReadAt] = useState<string | null>(null);
  const [checkedAt, setCheckedAt] = useState<string | null>(null);

  async function select(id: string, refresh = false, background = false) {
    if (
      background &&
      (writing.current ||
        readRequired.current ||
        requests.current.capture().id !== id)
    )
      return;
    const previous = requests.current.capture().id;
    const ticket = background
      ? requests.current.capture().sequence
      : requests.current.select(id);
    const sequence = ++readSequence.current;
    if (!background) {
      setSelectedId(id);
      setError(null);
      setLoading(true);
      if (previous !== id) {
        currentView.current = null;
        setView(null);
        setSummary(null);
        setReadAt(null);
        setCheckedAt(null);
        setMustRead(false);
        readRequired.current = false;
      }
    }
    try {
      const response = await read<TaskRecord>(
        "/api/tasks/" + encodeURIComponent(id) + (refresh ? "?refresh=1" : ""),
      );
      if (
        requests.current.current(ticket, id) &&
        sequence === readSequence.current
      ) {
        if (
          !background ||
          !sameTaskVersion(currentView.current, response.result)
        ) {
          currentView.current = response.result;
          setView(response.result);
        }
        setSummary(response.summary);
        setReadAt(new Date().toISOString());
        setError(null);
        if (refresh) setCheckedAt(new Date().toISOString());
        if (!background) {
          setMustRead(false);
          readRequired.current = false;
        }
      }
    } catch (error) {
      if (
        requests.current.current(ticket, id) &&
        sequence === readSequence.current
      )
        setError(error instanceof Error ? error : new Error(String(error)));
    } finally {
      if (
        requests.current.current(ticket, id) &&
        sequence === readSequence.current
      )
        setLoading(false);
    }
  }

  function clear() {
    requests.current.select("");
    readSequence.current++;
    currentView.current = null;
    setSelectedId("");
    setView(null);
    setSummary(null);
    setLoading(false);
    setError(null);
    setMustRead(false);
    readRequired.current = false;
    setReadAt(null);
    setCheckedAt(null);
  }

  async function mutate(
    tool: string,
    fields: Record<string, unknown>,
  ): Promise<MutationResult> {
    const view = currentView.current;
    if (
      !view ||
      view.identity.task_id !== requests.current.capture().id ||
      writing.current ||
      readRequired.current
    )
      return {
        ok: false,
        error: new Error(
          "Update task records before submitting another action.",
        ),
      };
    const selection = requests.current.capture();
    const args = { ...identity(view), ...fields };
    readSequence.current++;
    writing.current = true;
    setSaving(true);
    setLoading(false);
    setError(null);
    try {
      const next = await callTask(tool, args);
      if (requests.current.current(selection.sequence, selection.id)) {
        currentView.current = next.result;
        setView(next.result);
        setSummary(next.summary);
        setReadAt(new Date().toISOString());
        setCheckedAt(new Date().toISOString());
      }
      return { ok: true };
    } catch (value) {
      const error = value instanceof Error ? value : new Error(String(value));
      if (
        requests.current.current(selection.sequence, selection.id) &&
        error instanceof SavedStateRequired
      ) {
        setMustRead(true);
        readRequired.current = true;
        setError(error);
      }
      return { ok: false, error };
    } finally {
      writing.current = false;
      setSaving(false);
    }
  }

  return {
    selectedId,
    view,
    summary,
    loading,
    saving,
    busy: loading || saving,
    mutationBlocked: loading || saving || mustRead,
    mustRead,
    error,
    readAt,
    checkedAt,
    select,
    clear,
    mutate,
  };
}
