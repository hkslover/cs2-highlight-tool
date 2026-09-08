export type SnapshotSource = "event" | "snapshot";

export interface SnapshotState<T> {
  value: T;
  latestUpdatedAt: number;
  latestSource?: SnapshotSource;
}

export interface TimestampedSnapshot {
  updated_at_ms: number;
}

function timestampOf(snapshot: TimestampedSnapshot): number {
  const value = Number(snapshot.updated_at_ms);
  return Number.isFinite(value) && value >= 0 ? value : 0;
}

/**
 * Apply an event or initial snapshot without allowing a delayed read to roll
 * back a newer event. Equal timestamps from an event win over a snapshot too.
 */
export function applyProductionSnapshot<T extends TimestampedSnapshot>(
  state: SnapshotState<T>,
  next: T | null | undefined,
  source: SnapshotSource,
  empty: () => T,
): void {
  const snapshot = next || empty();
  const incomingUpdatedAt = timestampOf(snapshot);

  if (incomingUpdatedAt < state.latestUpdatedAt) return;
  if (
    source === "snapshot" &&
    state.latestSource === "event" &&
    incomingUpdatedAt <= state.latestUpdatedAt
  ) {
    return;
  }

  state.value = snapshot;
  state.latestUpdatedAt = incomingUpdatedAt;
  state.latestSource = source;
}

