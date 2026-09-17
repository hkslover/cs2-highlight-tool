import {
  buildRecordedViewIndex,
  pendingSelectionsByDemo,
  projectPendingSelection,
  recordedViewsForKill,
  type RecordedSpecModes,
  type RecordedViewIndex,
  type RecordedViewStatus,
} from "../src/domains/production/recordedViews";
import type {
  DemoListEntry,
  DemoMaterialSelection,
  ProduceHistoryItem,
} from "../src/shared/types";

const history: ProduceHistoryItem[] = [];
const index: RecordedViewIndex = buildRecordedViewIndex(history);
const modes: RecordedSpecModes = { killer: 1, victim: 1 };
const status: RecordedViewStatus = recordedViewsForKill(
  index,
  "demo.dem",
  "kill-1",
  modes,
);
const selection = {} as DemoMaterialSelection;
const projected = projectPendingSelection(selection, "demo.dem", index, modes);
const entry = {} as DemoListEntry;
const pending = pendingSelectionsByDemo(
  [entry],
  () => [selection],
  index,
  modes,
);

void status;
void projected;
void pending;
