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
