import { ref, type Ref } from "vue";
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import { backend } from "@/shared/backend";
import {
  createProductionStore,
  type ProductionStoreCell,
} from "./storeCore";

const store = createProductionStore({
  createCell<T>(initial: T): ProductionStoreCell<T> {
    return ref(initial) as unknown as ProductionStoreCell<T>;
  },
  subscribe(eventName, callback) {
    return EventsOn(eventName, callback);
  },
  backend: {
    getWSState: () => backend.GetProduceWSState(),
    getQueueState: () => backend.GetProduceQueueState(),
    getTakeSnapshot: () => backend.GetProduceTakeSnapshot(),
    getTakeFiles: () => backend.GetProduceTakeFiles(),
    getHistorySnapshot: () => backend.GetProduceHistorySnapshot(),
  },
});

export const initProductionState = store.initProductionState;
export const initProduceHistory = store.initProduceHistory;
export const disposeProductionState = store.disposeProductionState;

export function useProductionState() {
  return {
    wsState: store.wsState as Readonly<Ref<typeof store.wsState.value>>,
    queueState: store.queueState as Readonly<Ref<typeof store.queueState.value>>,
    takeSnapshot: store.takeSnapshot as Readonly<Ref<typeof store.takeSnapshot.value>>,
    takeFiles: store.takeFiles as Readonly<Ref<typeof store.takeFiles.value>>,
    historySnapshot: store.historySnapshot as Readonly<Ref<typeof store.historySnapshot.value>>,
    initializationError: store.initializationError as Readonly<Ref<string>>,
    initialized: store.initialized as Readonly<Ref<boolean>>,
    historyInitialized: store.historyInitialized as Readonly<Ref<boolean>>,
    initProductionState,
    initProduceHistory,
    disposeProductionState,
  };
}
