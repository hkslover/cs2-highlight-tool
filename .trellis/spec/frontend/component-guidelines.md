# Component Guidelines

> Vue 3 component conventions, Naive UI usage, template patterns.

---

## Vue Version and API Style

- **Vue 3** with Composition API exclusively
- All components use **`<script setup lang="ts">** — no Options API
- No `defineComponent()` wrapper needed in `<script setup>`

```vue
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
// no export default, no defineComponent()
</script>
```

---

## Vue API Import Principle

All Vue Composition API functions **must be explicitly imported** from `"vue"` in every `<script setup>` block or TypeScript module that uses them. Do **not** rely on `unplugin-auto-import`'s global declarations for Vue APIs.

### Why

The project uses `unplugin-auto-import` to automatically inject Vue API imports at build time. However, its global type declarations (`*.d.ts` globals) **may not resolve reliably** when TypeScript's `strict` mode is enabled, particularly on **Windows** environments. This manifests as build errors such as:

```
TS2304: Cannot find name 'nextTick'
```

These errors block production builds on Windows and are non-deterministic — they depend on the TypeScript language service's ability to resolve global declarations, which varies across platforms and tooling configurations.

### Correct pattern

```vue
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
// Explicit imports — always works cross-platform

const count = ref(0);
const doubled = computed(() => count.value * 2);

onMounted(() => {
  console.log("mounted");
});

await nextTick();
</script>
```

### Wrong pattern

```vue
<script setup lang="ts">
// ❌ No import from "vue" — relies on unplugin-auto-import globals
// This may fail on Windows with TS2304

const count = ref(0);
await nextTick();
</script>
```

> **Note:** This rule applies only to **Vue Composition API functions** (`ref`, `computed`, `nextTick`, `onMounted`, `watch`, etc.). Naive UI components are auto-registered by a different mechanism (`unplugin-vue-components`) and must **not** be manually imported — that rule remains unchanged.

## Template Conventions

### Structural Order

```vue
<template>
  <div class="feature-page">           <!-- Root element: kebab-case class matching feature -->
    <n-card :bordered="true">          <!-- Naive UI card as primary container -->
      <!-- ... -->
    </n-card>
  </div>
</template>

<script setup lang="ts">
// imports → composable calls → computed/watchers → lifecycle → methods
</script>

<style scoped>
/* Scoped styles only — no global styles outside App level */
</style>
```

### Scoped Styles

All component styles use `<style scoped>` — **no global styles** except in `App.vue` or root shell components. Use CSS classes (not inline styles) for layout and theming.

**Theme colors** are used directly as hex values (no CSS variables):
```css
.produce-page {
  background: #181b19;
}
.material-row {
  border: 1px solid #2f3631;
  border-radius: 8px;
}
```

### Responsive Patterns

Media queries are used inline in component scoped styles:
```css
@media (max-width: 980px) {
  .material-row {
    flex-direction: column;
  }
}
```

---

## Naive UI Usage

The project uses **Naive UI** as the component library. Component imports are auto-registered by `unplugin-vue-components` — **do not import Naive UI components manually**.

### Components used in the project

| Naive UI Component | Usage Pattern |
|--------------------|---------------|
| `n-card` | Primary page container, bordered by default |
| `n-button` | Actions with variant modifiers (`type="primary|warning|success|default"`) |
| `n-tag` | Status badges, version labels, view labels |
| `n-space` | Flexbox layout helper with `vertical` and `:size` props |
| `n-collapse` / `n-collapse-item` | Expandable section groups |
| `n-empty` | Empty state placeholders with `description` prop |
| `n-alert` | Warning/error/info banners |
| `n-dialog` (via `useDialog()`) | Confirmation dialogs |
| `n-message` (via `useMessage()`) | Toast notifications |
| `n-input` / `n-select` / `n-slider` | Form controls in settings pages |
| `n-switch` | Toggle controls |
| `n-spin` | Loading indicator |
| `n-checkbox` | Selection controls |
| `n-upload` | File import UI |

### Tooltip Content with Visual Demonstrations

When a tooltip needs to explain a feature with a static screenshot, keep the localized explanatory text and image in the tooltip content, import the image from `src/assets/images/`, and constrain the image responsively so it cannot overflow the overlay.

```vue
<n-tooltip trigger="hover" placement="top">
  <template #trigger><span class="hint-dot">?</span></template>
  <div class="setting-hint-content">
    <span>{{ t("main.settings.feature_hint") }}</span>
    <img :src="featureDemoImage" alt="" class="feature-demo-image" />
  </div>
</n-tooltip>
```

```css
.setting-hint-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.feature-demo-image {
  display: block;
  height: auto;
  max-width: min(360px, 70vw);
  width: 100%;
}
```

Use the conditional image only for the tooltip that owns the demonstration; other text-only hints should remain unchanged.

### Naive UI Composition API Hooks

```ts
const message = useMessage();   // Toast notifications
const dialog = useDialog();     // Confirmation dialogs
```

These must be called **inside `setup()` scope** (inside `<script setup>`).

### Naive UI Props

- `:bordered="true"` — commonly used on `n-card` (explicit binding, not shorthand)
- `size="small|tiny"` — used to reduce component footprint in dense layouts
- `type="success|warning|error|info|default"` — semantic variant for buttons, tags, alerts
- `:loading` — boolean binding for async action buttons
- `:disabled` — boolean binding for conditional enable/disable

---

## Composable Pattern

All features use composables for state and logic. The standard pattern:

```ts
// composables/useFeatureName.ts
import { computed, ref } from "vue";
import { t } from "@/shared/i18n";
import type { SomeType } from "@/shared/types";

// Module-level reactive state (see state-management.md)
const someState = ref<SomeType[]>([]);

export function useFeatureName() {
  // Local derived state
  const derived = computed(() => someState.value.filter(/* ... */));

  // Methods
  function doSomething() {
    // ...
  }

  return {
    derived,
    doSomething,
  };
}
```

### Returns Pattern

Composables return an object with all public bindings:

```ts
return {
  t,
  busy,
  tasks,
  statusText,
  statusTagType,
  retry,
  reinstall,
  enterMain,
  // ...
};
```

---

## Wails Backend Calling Pattern

All backend calls go through a `callBackend` helper:

```ts
async function callBackend(method: string, ...args: unknown[]) {
  const api = window.go?.app?.App as Record<string, (...a: unknown[]) => Promise<unknown>> | undefined;
  const fn = api?.[method];
  if (!fn) throw new Error(`Wails API not loaded: ${method}`);
  return fn(...args);
}
```

This pattern is **duplicated in each composable** that needs backend access — not centralized in a shared utility. Always use this exact pattern when adding a new composable that calls backend methods.

### Usage example

```ts
const result = await callBackend<ProduceQueueState>("GetProduceQueueState");
```

---

## Lifecycle

- `onMounted` — used for initial data fetching, event subscriptions
- `onBeforeUnmount` — used for cleanup (unsubscribe events, cancel timers)
- `watch` — used for reactive side effects (e.g., auto-expand panels when data changes)

```ts
onMounted(async () => {
  queueState.value = await callBackend<ProduceQueueState>("GetProduceQueueState");
  offEventHandlers.push(
    EventsOn("produce_ws_state_changed", (next) => { wsState.value = next; }),
  );
});

onBeforeUnmount(() => {
  for (const off of offEventHandlers) { off(); }
});
```

---

## Vue Router Usage

```ts
import { useRouter } from "vue-router";
const router = useRouter();

// Programmatic navigation
void router.push("/edit");
```

Routes are lazy-loaded via dynamic `import()`:

```ts
component: () => import("@/features/import/pages/ImportPage.vue"),
```

Navigation guards are not used — route changes are driven by user clicks and programmatic navigation.

---

## Infinite Scroll Pattern with n-data-table

When implementing infinite (lazy) scroll pagination with `<n-data-table flex-height>`, use the **two-path pattern**: auto-fill (no overflow yet) and scroll-triggered (overflow exists) are **fully separate code paths**.

### Why this pattern

- Does **not** depend on Naive UI internal CSS classes (`.n-scrollbar-container`, `.n-scrollbar-content`)
- No dynamic DOM insertion/cleanup needed
- Handles both user-scroll-to-bottom and content-not-filling-viewport cases
- Uses `ResizeObserver` to re-check when container size changes

### Critical: findScrollContainer must pick largest clientHeight

> **Gotcha**: Naive UI's `<n-data-table flex-height>` renders **two** `overflow-y: auto/scroll` containers — the header scroller (short, ~30px) and the body scroller (tall, fills flex space). The header appears **first** in the DOM. If you take the first match, `scrollHeight` always ≈ `clientHeight` (~30px) so `hasVerticalOverflow` always returns `false`, and auto-fill never stops.
>
> **Always pick the container with the largest `clientHeight`** — that is the body scroller.

```ts
// ❌ WRONG — takes the first overflow container (header scroller, ~30px)
function findScrollContainer(): HTMLElement | null {
  const candidates = root.querySelectorAll("div");
  for (let i = 0; i < candidates.length; i++) {
    const el = candidates[i];
    const style = getComputedStyle(el);
    if (style.overflowY === "auto" || style.overflowY === "scroll") return el; // header found first!
  }
  return null;
}

// ✅ CORRECT — picks the tallest container (body scroller)
function findScrollContainer(): HTMLElement | null {
  const root = nTableRef.value?.$el;
  if (!root) return null;
  let best: HTMLElement | null = null;
  let bestClientHeight = 0;
  const candidates = root.querySelectorAll("div");
  for (let i = 0; i < candidates.length; i++) {
    const el = candidates[i] as HTMLElement;
    const style = getComputedStyle(el);
    if (
      (style.overflowY === "auto" || style.overflowY === "scroll") &&
      el !== document.body && el !== document.documentElement &&
      el.clientHeight > bestClientHeight
    ) {
      best = el;
      bestClientHeight = el.clientHeight;
    }
  }
  return best;
}
```

### Implementation sketch

```ts
const props = defineProps<{
  rows: unknown[];
  loading: boolean;
  loadingMore: boolean;
  canLoadMore: boolean;
}>();

const emit = defineEmits<{ (e: "load-more"): void }>();

const nTableRef = ref<any>(null);
const SCROLL_BOTTOM_THRESHOLD = 24;

let scrollContainerEl: HTMLElement | null = null;
let loadMorePending = false;
let resizeObserver: ResizeObserver | null = null;

function hasVerticalOverflow(container: HTMLElement): boolean {
  return container.scrollHeight > container.clientHeight + 1;
}

function isNearScrollBottom(container: HTMLElement): boolean {
  return container.scrollTop + container.clientHeight >= container.scrollHeight - SCROLL_BOTTOM_THRESHOLD;
}

function emitLoadMore(): void {
  if (!scrollContainerEl) return;
  if (loadMorePending || props.loading || props.loadingMore || !props.canLoadMore) return;
  loadMorePending = true;
  emit("load-more");
  requestAnimationFrame(() => { loadMorePending = false; });
}

// Path 1 — Auto-fill: fires only when there is NO overflow yet
function maybeAutoFill(): void {
  if (!scrollContainerEl || props.rows.length === 0) return;
  if (hasVerticalOverflow(scrollContainerEl)) return; // overflow exists → stop
  emitLoadMore();
}

// Path 2 — Scroll-triggered: fires only when overflow EXISTS and user is near bottom
function maybeLoadMoreFromScroll(): void {
  if (!scrollContainerEl) return;
  if (!hasVerticalOverflow(scrollContainerEl)) return; // no overflow → stop
  if (!isNearScrollBottom(scrollContainerEl)) return;
  emitLoadMore();
}

// ensureScrollContainer: don't cache a zero-height container (layout not done yet)
function ensureScrollContainer(): void {
  if (scrollContainerEl && scrollContainerEl.clientHeight > 0) return;
  const found = findScrollContainer();
  if (!found || found.clientHeight === 0) return;
  if (found === scrollContainerEl) return;
  if (resizeObserver) { resizeObserver.disconnect(); resizeObserver = null; }
  scrollContainerEl = found;
  attachResizeObserver();
}

// handleTableScroll: always trust the event target (guaranteed to be correct body scroller)
function handleTableScroll(event: Event): void {
  const target = event.target as HTMLElement | null;
  if (!target) return;
  if (scrollContainerEl !== target) {
    if (resizeObserver) { resizeObserver.disconnect(); resizeObserver = null; }
    scrollContainerEl = target;
    attachResizeObserver();
  }
  maybeLoadMoreFromScroll();
}

function attachResizeObserver(): void {
  if (!scrollContainerEl || resizeObserver) return;
  resizeObserver = new ResizeObserver(() => { maybeAutoFill(); });
  resizeObserver.observe(scrollContainerEl);
}

watch(() => props.rows.length, () => {
  void nextTick(() => { ensureScrollContainer(); maybeAutoFill(); });
}, { flush: "post" });

watch(() => [props.canLoadMore, props.loading, props.loadingMore] as const, () => {
  void nextTick(() => { ensureScrollContainer(); maybeAutoFill(); });
}, { flush: "post" });

onMounted(() => {
  void nextTick(() => { ensureScrollContainer(); maybeAutoFill(); });
});

onBeforeUnmount(() => {
  if (resizeObserver) { resizeObserver.disconnect(); resizeObserver = null; }
});
```

### Template binding

```vue
<n-data-table
  ref="nTableRef"
  :data="rows"
  :loading="loading"
  :on-scroll="handleTableScroll"
  :flex-height="true"
>
```

### Key points

- **Two paths are mutually exclusive**: `maybeAutoFill` runs only when `!hasVerticalOverflow`; `maybeLoadMoreFromScroll` runs only when `hasVerticalOverflow` is true
- **findScrollContainer picks largest `clientHeight`** — not first — to skip the header scroller
- **ensureScrollContainer rejects zero-height containers** — on first mount the flex layout may not be computed yet; re-try on next data change
- **handleTableScroll always corrects `scrollContainerEl`** — the scroll event fires on the actual body scroller; use it to fix any wrong initial detection
- `loadMorePending` mutex + `requestAnimationFrame` prevents duplicate emissions within the same frame
- The parent composable's `loading`/`loadingMore`/`canLoadMore` serve as the ultimate backstop
- The `.empty` slot override is needed to prevent the empty-state block from breaking flex-height layout

---

## What Not to Do

- ❌ **Do not manually import Naive UI components** — unplugin-vue-components auto-registers them
- ❌ **Do not rely on unplugin-auto-import for Vue APIs** — always explicitly import Vue APIs (`computed`, `nextTick`, `onBeforeUnmount`, `onMounted`, `ref`, `watch`, etc.) from `"vue"`. Global auto-import declarations may not resolve reliably on Windows TypeScript strict builds (TS2304).
- ❌ **Do not use Options API** (`export default { data() {}, methods: {} }`)
- ❌ **Do not use global styles outside App.vue** — use `scoped` always
- ❌ **Do not use inline `window.go.app.App.xxx()`** — always go through `callBackend()` for error handling
- ❌ **Do not call `useMessage()` / `useDialog()` outside `<script setup>`** — they require injection context
- ❌ **Do not use `@/` in relative imports within the same feature** — use `./` or `../`
