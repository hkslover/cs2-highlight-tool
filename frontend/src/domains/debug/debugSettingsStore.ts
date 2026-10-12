import { ref, watch, type Ref } from "vue";
import { backend } from "../../shared/backend/adapter.js";
import {
  createDebugSettingsCore,
  type DebugSettingsCell,
} from "./debugSettingsCore.js";
import type {
  ClipSettings,
  DebugConsolePlan,
  DebugSettingsDraft,
} from "../../shared/types/index.js";

const core = createDebugSettingsCore({
  createCell<T>(initial: T): DebugSettingsCell<T> {
    // SAFETY: a Vue Ref is structurally a mutable value cell and the core only
    // reads/writes `.value`, so erasing the generic wrapper is sound here.
    return ref(initial) as unknown as DebugSettingsCell<T>;
  },
  backend: {
    getClipSettings: () => backend.GetClipSettings(),
    getConsolePlan: (draft: DebugSettingsDraft) => backend.GetDebugConsolePlan(draft),
  },
});

const DEBUG_PLAN_REFRESH_DELAY_MS = 150;

let planTimer: ReturnType<typeof setTimeout> | null = null;
// Explicit plan refreshes (apply actions, load, reload) are serialized: the
// core keeps only the newest request version, so letting a later explicit call
// bump it would turn a waiting apply into a stale `null` result that reads as
// "no plan". A draft edit that arrives while explicit work is queued or in
// flight only marks the preview as pending; it is rebuilt once the queue
// drains. `schedulerGeneration` drops timers and queue bookkeeping from a
// workspace that was already disposed.
let explicitPlanTail: Promise<void> = Promise.resolve();
let explicitPlanPending = 0;
let previewRefreshPending = false;
let schedulerGeneration = 0;

function clearPlanTimer(): void {
  if (planTimer != null) {
    clearTimeout(planTimer);
    planTimer = null;
  }
}

function schedulePreviewPlanRefresh(): void {
  if (explicitPlanPending > 0) {
    previewRefreshPending = true;
    return;
  }
  const generation = schedulerGeneration;
  clearPlanTimer();
  planTimer = setTimeout(() => {
    planTimer = null;
    if (generation !== schedulerGeneration) return;
    if (explicitPlanPending > 0) {
      previewRefreshPending = true;
      return;
    }
    void core.refreshPlan();
  }, DEBUG_PLAN_REFRESH_DELAY_MS);
}

/**
 * Explicit apply entry: cancel a pending preview debounce and build a plan for
 * the current draft. Requests run one after another because the core only
 * keeps the newest plan request version; otherwise a settings reload issued
 * while an apply waits would steal the apply's result and the user's command
 * would silently not be sent.
 */
async function applyDebugPlan(): Promise<DebugConsolePlan | null> {
  clearPlanTimer();
  const generation = schedulerGeneration;
  explicitPlanPending += 1;
  const task = explicitPlanTail.then(() =>
    generation === schedulerGeneration ? core.refreshPlan() : null,
  );
  explicitPlanTail = task.then(
    () => undefined,
    () => undefined,
  );
  try {
    return await task;
  } finally {
    if (generation === schedulerGeneration) {
      explicitPlanPending -= 1;
      if (explicitPlanPending === 0 && previewRefreshPending) {
        previewRefreshPending = false;
        schedulePreviewPlanRefresh();
      }
    }
  }
}

// A draft edit only rebuilds the preview/plan; it never calls a settings save
// method. The initial load assignment is skipped because `loaded` flips after
// the draft is replaced, and the page refreshes the plan explicitly then.
watch(
  core.draft as Ref<DebugSettingsDraft>,
  () => {
    if (!core.loaded.value) return;
    schedulePreviewPlanRefresh();
  },
  { deep: true, flush: "sync" },
);

/**
 * Initialize the draft from the formal settings when this workspace has no
 * draft yet, then make sure a console plan exists. A route switch must not
 * reset the session's debug choices, so an already-initialized draft is kept.
 */
async function loadDebugSettings(): Promise<boolean> {
  const ok = await core.load();
  if (!ok) return false;
  if (core.plan.value === null) {
    await applyDebugPlan();
  }
  return true;
}

/** The explicit reload button always replaces the draft from formal settings. */
async function reloadDebugSettings(): Promise<boolean> {
  const ok = await core.load(true);
  if (!ok) return false;
  await applyDebugPlan();
  return true;
}

/** Drop local draft facts and timers when the workspace changes. */
export function resetDebugSettingsForWorkspace(): void {
  schedulerGeneration += 1;
  clearPlanTimer();
  // The new workspace must not wait for a late response from the old queue.
  // Generation checks still discard the old queue's results and bookkeeping.
  explicitPlanTail = Promise.resolve();
  explicitPlanPending = 0;
  previewRefreshPending = false;
  core.dispose();
}

export function useDebugSettingsDraft() {
  return {
    draft: core.draft as Readonly<Ref<DebugSettingsDraft>>,
    sourceSettings: core.sourceSettings as Readonly<Ref<ClipSettings | null>>,
    plan: core.plan as Readonly<Ref<DebugConsolePlan | null>>,
    loaded: core.loaded as Readonly<Ref<boolean>>,
    loading: core.loading as Readonly<Ref<boolean>>,
    planLoading: core.planLoading as Readonly<Ref<boolean>>,
    errorMessage: core.errorMessage as Readonly<Ref<string>>,
    planErrorMessage: core.planErrorMessage as Readonly<Ref<string>>,
    loadDebugSettings,
    reloadDebugSettings,
    applyDebugPlan,
    patchDraft: core.patchDraft,
  };
}
