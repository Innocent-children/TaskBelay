import type { View } from "./api";

export function sameTaskVersion(current: View | null, next: View) {
  return (
    !!current &&
    current.identity.task_id === next.identity.task_id &&
    current.identity.revision === next.identity.revision &&
    current.digests.authorization === next.digests.authorization &&
    current.digests.content === next.digests.content &&
    current.digests.plan === next.digests.plan
  );
}

/**
 * A late read or write response may update only the selection that issued it.
 * Operation IDs are owned by callers and are never regenerated for an automatic retry.
 */
export class SelectionRequests {
  private sequence = 0;
  private selected = "";
  select(id: string) {
    this.selected = id;
    return ++this.sequence;
  }
  current(sequence: number, id: string) {
    return sequence === this.sequence && id === this.selected;
  }
  capture() {
    return { sequence: this.sequence, id: this.selected };
  }
}
