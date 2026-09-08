import { computed, ref, type ComputedRef, type Ref } from "vue";
import { EventsOn } from "../../../wailsjs/runtime/runtime.js";
import { backend } from "../../shared/backend/index.js";
import type { ComposeProgressMessage, ProduceHistoryItem } from "@/shared/types";
import type {
  EditConcatRequestPayload,
  EditConcatTransitionPayload,
  EditSequenceItem,
  EditTransitionMode,
} from "./types";

const COMPOSE_PROGRESS_EVENT = "compose_progress";

export interface EditDomainRuntime {
  onComposeProgress(handler: (next: ComposeProgressMessage) => void): () => void;
  concatEditClips(request: EditConcatRequestPayload): Promise<string>;
}

export interface EditDomainState {
  sequenceItems: Readonly<Ref<EditSequenceItem[]>>;
  exporting: Readonly<Ref<boolean>>;
  exportError: Readonly<Ref<string>>;
  exportPath: Readonly<Ref<string>>;
  transitionMode: Readonly<Ref<EditTransitionMode>>;
  transitionDuration: Readonly<Ref<number>>;
  totalDuration: ComputedRef<number>;
  composeProgress: Readonly<Ref<ComposeProgressMessage>>;
  composePercent: ComputedRef<number>;
}

export interface EditDomainController extends EditDomainState {
  init(): void;
  dispose(): void;
  addSequenceItem(item: ProduceHistoryItem, duration: number): void;
  moveSequenceItemUp(index: number): void;
  moveSequenceItemDown(index: number): void;
  removeSequenceItem(index: number): void;
  clearSequence(): void;
  setTransitionMode(mode: EditTransitionMode): void;
  setTransitionDuration(duration: number): void;
  setExportError(value: string): void;
  setExportPath(value: string): void;
  clearExportError(): void;
  buildConcatRequest(): EditConcatRequestPayload;
  exportSequence(): Promise<string | null>;
}

export interface EditDomainLifecycle {
  init(): void;
  dispose(): void;
}

const initialComposeProgress = (): ComposeProgressMessage => ({
  active: false,
  percent: 0,
  current_step: "",
  elapsed_ms: 0,
  error: "",
});

function generateId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function normalizeComposeProgress(next: ComposeProgressMessage | null | undefined): ComposeProgressMessage {
  const source = next || initialComposeProgress();
  const percent = Number(source.percent);
  const elapsed = Number(source.elapsed_ms);
  return {
    active: !!source.active,
    percent: Number.isFinite(percent) ? Math.max(0, Math.min(100, percent)) : 0,
    current_step: String(source.current_step || ""),
    elapsed_ms: Number.isFinite(elapsed) ? elapsed : 0,
    error: String(source.error || ""),
  };
}

function defaultRuntime(): EditDomainRuntime {
  return {
    onComposeProgress(handler) {
      return EventsOn(COMPOSE_PROGRESS_EVENT, handler);
    },
    concatEditClips(request: EditConcatRequestPayload): Promise<string> {
      return backend.ConcatEditClips(request);
    },
  };
}

export function buildEditConcatRequest(
  sequenceItems: readonly EditSequenceItem[],
  transitionMode: EditTransitionMode,
  transitionDuration: number,
): EditConcatRequestPayload {
  const clips = sequenceItems.map((item) => ({
    video_path: item.videoPath,
    duration: item.duration,
  }));
  const transitions: EditConcatTransitionPayload[] = [];
  if (transitionMode === "fade" && sequenceItems.length > 1) {
    for (let index = 0; index < sequenceItems.length - 1; index++) {
      transitions.push({
        type: "fade",
        duration: transitionDuration,
        after_index: index,
      });
    }
  }
  return { clips, transitions };
}

export function createEditDomain(runtime: EditDomainRuntime = defaultRuntime()): EditDomainController {
  const sequenceItems = ref<EditSequenceItem[]>([]);
  const exporting = ref(false);
  const exportError = ref("");
  const exportPath = ref("");
  const transitionMode = ref<EditTransitionMode>("none");
  const transitionDuration = ref(0.3);
  const composeProgress = ref<ComposeProgressMessage>(initialComposeProgress());
  const totalDuration = computed(() =>
    sequenceItems.value.reduce((sum, item) => sum + item.duration, 0),
  );
  const composePercent = computed(() => composeProgress.value.percent);

  let initialized = false;
  let offComposeProgress: (() => void) | undefined;
  let lifecycleEpoch = 0;
  let activeExportEpoch: number | undefined;

  function applyComposeProgress(next: ComposeProgressMessage) {
    const normalized = normalizeComposeProgress(next);
    composeProgress.value = normalized;
    if (normalized.error) {
      exportError.value = normalized.error;
    }
  }

  function init() {
    if (initialized) return;
    const nextEpoch = lifecycleEpoch + 1;
    if (exporting.value && activeExportEpoch !== lifecycleEpoch) {
      // A previous app instance was disposed while its backend call was still
      // running. Its continuation must not keep the new instance busy.
      exporting.value = false;
      composeProgress.value = initialComposeProgress();
      exportError.value = "";
      exportPath.value = "";
      activeExportEpoch = undefined;
    }
    // Bind the event callback to this lifecycle. An event that was queued
    // before dispose can still invoke the old handler after a new init.
    lifecycleEpoch = nextEpoch;
    const off = runtime.onComposeProgress((next) => {
      if (lifecycleEpoch !== nextEpoch) return;
      applyComposeProgress(next);
    });
    offComposeProgress = off;
    initialized = true;
  }

  function dispose() {
    initialized = false;
    lifecycleEpoch += 1;
    offComposeProgress?.();
    offComposeProgress = undefined;
  }

  function addSequenceItem(item: ProduceHistoryItem, duration: number) {
    sequenceItems.value.push({
      id: generateId(),
      historyItem: item,
      videoPath: item.video_path,
      duration,
    });
  }

  function moveSequenceItemUp(index: number) {
    if (index <= 0 || index >= sequenceItems.value.length) return;
    const next = sequenceItems.value.slice();
    const [moved] = next.splice(index, 1);
    next.splice(index - 1, 0, moved);
    sequenceItems.value = next;
  }

  function moveSequenceItemDown(index: number) {
    if (index < 0 || index >= sequenceItems.value.length - 1) return;
    const next = sequenceItems.value.slice();
    const [moved] = next.splice(index, 1);
    next.splice(index + 1, 0, moved);
    sequenceItems.value = next;
  }

  function removeSequenceItem(index: number) {
    if (index < 0 || index >= sequenceItems.value.length) return;
    const next = sequenceItems.value.slice();
    next.splice(index, 1);
    sequenceItems.value = next;
  }

  function clearSequence() {
    sequenceItems.value = [];
    exportError.value = "";
    exportPath.value = "";
  }

  function setTransitionMode(mode: EditTransitionMode) {
    transitionMode.value = mode;
  }

  function setTransitionDuration(duration: number) {
    transitionDuration.value = duration;
  }

  function setExportError(value: string) {
    exportError.value = value;
  }

  function setExportPath(value: string) {
    exportPath.value = value;
  }

  function clearExportError() {
    exportError.value = "";
  }

  function buildConcatRequest(): EditConcatRequestPayload {
    return buildEditConcatRequest(
      sequenceItems.value,
      transitionMode.value,
      transitionDuration.value,
    );
  }

  async function exportSequence(): Promise<string | null> {
    if (!sequenceItems.value.length || exporting.value) return null;

    try {
      init();
    } catch {
      // Backend calls remain usable when the host event bridge is not ready;
      // the next app init can retry the subscription.
    }
    const operationEpoch = lifecycleEpoch;
    activeExportEpoch = operationEpoch;
    const request = buildConcatRequest();
    exporting.value = true;
    exportError.value = "";
    exportPath.value = "";
    composeProgress.value = {
      ...initialComposeProgress(),
      active: true,
    };

    try {
      const result = await runtime.concatEditClips(request);
      if (operationEpoch !== lifecycleEpoch) return result;
      exportPath.value = result;
      exportError.value = "";
      composeProgress.value = {
        ...composeProgress.value,
        active: false,
        percent: 100,
        error: "",
      };
      return result;
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      if (operationEpoch !== lifecycleEpoch) return null;
      exportError.value = msg;
      composeProgress.value = {
        ...composeProgress.value,
        active: false,
        error: msg,
      };
      return null;
    } finally {
      if (operationEpoch === lifecycleEpoch) {
        exporting.value = false;
        activeExportEpoch = undefined;
      }
    }
  }

  return {
    sequenceItems,
    exporting,
    exportError,
    exportPath,
    transitionMode,
    transitionDuration,
    totalDuration,
    composeProgress,
    composePercent,
    init,
    dispose,
    addSequenceItem,
    moveSequenceItemUp,
    moveSequenceItemDown,
    removeSequenceItem,
    clearSequence,
    setTransitionMode,
    setTransitionDuration,
    setExportError,
    setExportPath,
    clearExportError,
    buildConcatRequest,
    exportSequence,
  };
}

export const editDomain = createEditDomain();

export function initEditDomain(): void {
  editDomain.init();
}

export function disposeEditDomain(): void {
  editDomain.dispose();
}

/** App-shell integration point. The domain owns the subscription until this is disposed. */
export const editDomainLifecycle: EditDomainLifecycle = {
  init: initEditDomain,
  dispose: disposeEditDomain,
};

export function useEditDomain(): EditDomainController {
  return editDomain;
}
