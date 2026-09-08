import {
  createProductionStore,
  type ProductionStoreBackend,
} from "../src/domains/production/storeCore";

const backend = {} as ProductionStoreBackend;
const store = createProductionStore({
  backend,
  createCell: <T>(value: T) => ({ value }),
  subscribe: (_eventName, _callback) => () => undefined,
});

store.initProductionState();
store.initProduceHistory();
store.disposeProductionState();

