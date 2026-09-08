import {
  applyProductionSnapshot,
  type SnapshotSource,
  type SnapshotState,
  type TimestampedSnapshot,
} from "./snapshot";
import type {
  ProduceHistorySnapshot,
  ProduceQueueState,
  ProduceTakeFileSnapshot,
  ProduceTakeStatusSnapshot,
  ProduceWSState,
} from "@/shared/types";

export type ProductionResourceKey = "ws" | "queue" | "takes" | "files" | "history";

export interface ProductionStoreBackend {
  getWSState: () => Promise<ProduceWSState>;
  getQueueState: () => Promise<ProduceQueueState>;
  getTakeSnapshot: () => Promise<ProduceTakeStatusSnapshot>;
  getTakeFiles: () => Promise<ProduceTakeFileSnapshot>;
  getHistorySnapshot: () => Promise<ProduceHistorySnapshot>;
}

export type ProductionEventSubscriber = (
  eventName: string,
  callback: (payload: unknown) => void,
) => () => void;

export interface ProductionStoreCell<T> {
  value: T;
}

export interface ProductionStoreOptions {
  backend: ProductionStoreBackend;
  subscribe: ProductionEventSubscriber;
  createCell: <T>(initial: T) => ProductionStoreCell<T>;
}

interface SnapshotResource<T extends TimestampedSnapshot> {
  eventName: string;
  readSnapshot: () => Promise<T>;
  state: ProductionStoreCell<T>;
  empty: () => T;
  initialized: boolean;
  initializing?: Promise<void>;
  unsubscribe?: () => void;
  generation: number;
  latestUpdatedAt: number;
  latestSource?: SnapshotSource;
}

interface SnapshotTypes {
  ws: ProduceWSState;
  queue: ProduceQueueState;
  takes: ProduceTakeStatusSnapshot;
  files: ProduceTakeFileSnapshot;
  history: ProduceHistorySnapshot;
}

const productionResourceKeys: ProductionResourceKey[] = ["ws", "queue", "takes", "files", "history"];

export function createProductionStore(options: ProductionStoreOptions) {
  const wsState = options.createCell<ProduceWSState>({
    address: "",
    connected: false,
    updated_at_ms: 0,
  });
  const queueState = options.createCell<ProduceQueueState>({
    running: false,
    total: 0,
    completed: 0,
    current_index: -1,
    pending_ack: false,
    updated_at_ms: 0,
  });
  const takeSnapshot = options.createCell<ProduceTakeStatusSnapshot>({
    items: [],
    total_takes: 0,
    started_takes: 0,
    completed_takes: 0,
    updated_at_ms: 0,
  });
  const takeFiles = options.createCell<ProduceTakeFileSnapshot>({
    items: [],
    updated_at_ms: 0,
  });
  const historySnapshot = options.createCell<ProduceHistorySnapshot>({
    items: [],
    updated_at_ms: 0,
  });
  const initialized = options.createCell(false);
  const historyInitialized = options.createCell(false);
  const initializationError = options.createCell("");

  const resources: {
    [K in ProductionResourceKey]: SnapshotResource<SnapshotTypes[K]>;
  } = {
    ws: {
      eventName: "produce_ws_state_changed",
      readSnapshot: options.backend.getWSState,
      state: wsState,
      empty: () => ({ address: "", connected: false, updated_at_ms: 0 }),
      initialized: false,
      generation: 0,
      latestUpdatedAt: 0,
    },
    queue: {
      eventName: "produce_queue_state_changed",
      readSnapshot: options.backend.getQueueState,
      state: queueState,
      empty: () => ({
        running: false,
        total: 0,
        completed: 0,
        current_index: -1,
        pending_ack: false,
        updated_at_ms: 0,
      }),
      initialized: false,
      generation: 0,
      latestUpdatedAt: 0,
    },
    takes: {
      eventName: "produce_take_status_changed",
      readSnapshot: options.backend.getTakeSnapshot,
      state: takeSnapshot,
      empty: () => ({
        items: [],
        total_takes: 0,
        started_takes: 0,
        completed_takes: 0,
        updated_at_ms: 0,
      }),
      initialized: false,
      generation: 0,
      latestUpdatedAt: 0,
    },
    files: {
      eventName: "produce_take_file_changed",
      readSnapshot: options.backend.getTakeFiles,
      state: takeFiles,
      empty: () => ({ items: [], updated_at_ms: 0 }),
      initialized: false,
      generation: 0,
      latestUpdatedAt: 0,
    },
    history: {
      eventName: "produce_history_changed",
      readSnapshot: options.backend.getHistorySnapshot,
      state: historySnapshot,
      empty: () => ({ items: [], updated_at_ms: 0 }),
      initialized: false,
      generation: 0,
      latestUpdatedAt: 0,
    },
  };

  let productionInitialization: Promise<void> | undefined;
  let historyInitialization: Promise<void> | undefined;
  let storeGeneration = 0;

  function refreshInitializedFlags(): void {
    initialized.value = productionResourceKeys.every((key) => resources[key].initialized);
    historyInitialized.value = resources.history.initialized;
  }

  function applySnapshot<K extends ProductionResourceKey>(
    resource: SnapshotResource<SnapshotTypes[K]>,
    next: SnapshotTypes[K] | null | undefined,
    source: SnapshotSource,
    generation: number,
  ): void {
    if (generation !== resource.generation) return;
    const state: SnapshotState<SnapshotTypes[K]> = {
      value: resource.state.value,
      latestUpdatedAt: resource.latestUpdatedAt,
      latestSource: resource.latestSource,
    };
    applyProductionSnapshot(state, next, source, resource.empty);
    resource.state.value = state.value;
    resource.latestUpdatedAt = state.latestUpdatedAt;
    resource.latestSource = state.latestSource;
  }

  function releaseResourceSubscription(resource: SnapshotResource<any>): void {
    resource.unsubscribe?.();
    resource.unsubscribe = undefined;
  }

  function initializeResource<K extends ProductionResourceKey>(
    resource: SnapshotResource<SnapshotTypes[K]>,
  ): Promise<void> {
    if (resource.initialized) return Promise.resolve();
    if (resource.initializing) return resource.initializing;

    const generation = resource.unsubscribe ? resource.generation : ++resource.generation;
    if (!resource.unsubscribe) {
      try {
        resource.unsubscribe = options.subscribe(resource.eventName, (payload) => {
          applySnapshot(resource, payload as SnapshotTypes[K], "event", generation);
        });
      } catch (error: unknown) {
        if (resource.generation === generation) {
          resource.initialized = false;
          refreshInitializedFlags();
        }
        return Promise.reject(error);
      }
    }

    let currentPromise: Promise<void>;
    // Start the read in the promise chain so a synchronous adapter/host
    // exception follows the same retryable path as a rejected Promise. This
    // also installs `resource.initializing` before the read is invoked.
    currentPromise = Promise.resolve()
      .then(() => resource.readSnapshot())
      .then((snapshot) => {
        applySnapshot(resource, snapshot, "snapshot", generation);
        if (resource.generation === generation) {
          resource.initialized = true;
          refreshInitializedFlags();
        }
      })
      .catch((error: unknown) => {
        if (resource.generation === generation) {
          resource.initialized = false;
          // Keep the event subscription. A retry can reuse it, and a live
          // event remains useful even while the initial read is unavailable.
          refreshInitializedFlags();
        }
        throw error;
      })
      .finally(() => {
        if (resource.initializing === currentPromise) resource.initializing = undefined;
      });
    resource.initializing = currentPromise;
    return currentPromise;
  }

  function initializeResources(keys: readonly ProductionResourceKey[]): Promise<void> {
    const pending = keys.map((key) => initializeResource(resources[key]));
    return Promise.all(pending).then(() => undefined);
  }

  function initProductionState(): Promise<void> {
    if (productionInitialization) return productionInitialization;
    const generation = storeGeneration;
    initializationError.value = "";
    let currentPromise: Promise<void>;
    currentPromise = initializeResources(productionResourceKeys)
      .catch((error: unknown) => {
        if (storeGeneration === generation) {
          initializationError.value = error instanceof Error ? error.message : String(error);
        }
        throw error;
      })
      .finally(() => {
        if (productionInitialization === currentPromise) productionInitialization = undefined;
      });
    productionInitialization = currentPromise;
    return currentPromise;
  }

  function initProduceHistory(): Promise<void> {
    if (historyInitialization) return historyInitialization;
    const generation = storeGeneration;
    initializationError.value = "";
    let currentPromise: Promise<void>;
    currentPromise = initializeResources(["history"])
      .catch((error: unknown) => {
        if (storeGeneration === generation) {
          initializationError.value = error instanceof Error ? error.message : String(error);
        }
        throw error;
      })
      .finally(() => {
        if (historyInitialization === currentPromise) historyInitialization = undefined;
      });
    historyInitialization = currentPromise;
    return currentPromise;
  }

  function disposeProductionState(): void {
    storeGeneration++;
    productionInitialization = undefined;
    historyInitialization = undefined;
    initializationError.value = "";
    for (const key of productionResourceKeys) {
      const resource = resources[key];
      resource.generation++;
      releaseResourceSubscription(resource);
      resource.initialized = false;
      resource.initializing = undefined;
      resource.latestUpdatedAt = 0;
      resource.latestSource = undefined;
      resource.state.value = resource.empty();
    }
    refreshInitializedFlags();
  }

  return {
    wsState,
    queueState,
    takeSnapshot,
    takeFiles,
    historySnapshot,
    initialized,
    historyInitialized,
    initializationError,
    initProductionState,
    initProduceHistory,
    disposeProductionState,
  };
}
