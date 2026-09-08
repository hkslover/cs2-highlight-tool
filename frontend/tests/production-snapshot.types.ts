import { applyProductionSnapshot, type SnapshotState } from "../src/domains/production/snapshot";

interface TestSnapshot {
  updated_at_ms: number;
  connected: boolean;
}

const state: SnapshotState<TestSnapshot> = {
  value: { updated_at_ms: 0, connected: false },
  latestUpdatedAt: 0,
};

applyProductionSnapshot(
  state,
  { updated_at_ms: 1, connected: true },
  "event",
  () => ({ updated_at_ms: 0, connected: false }),
);

