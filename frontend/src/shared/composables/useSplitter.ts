import { computed, onBeforeUnmount, ref, type Ref } from "vue";

export interface UseSplitterOptions {
  /** Initial ratio of left panel (0 to 1), default 0.5 */
  initialLeftRatio?: number;
  /** Minimum width in pixels for left panel, default 200 */
  minLeftPx?: number;
  /** Minimum width in pixels for right panel, default 200 */
  minRightPx?: number;
  /** Splitter handle width in pixels, default 12 */
  splitterWidthPx?: number;
}

export function useSplitter(
  containerRef: Ref<HTMLElement | null>,
  options: UseSplitterOptions = {},
) {
  const {
    initialLeftRatio = 0.5,
    minLeftPx = 200,
    minRightPx = 200,
    splitterWidthPx = 12,
  } = options;

  const isResizing = ref(false);
  const leftRatio = ref(initialLeftRatio);

  const leftPanelStyle = computed(() => ({
    flexBasis: `calc(${leftRatio.value * 100}% - ${leftRatio.value * splitterWidthPx}px)`,
    flexGrow: 0,
    flexShrink: 0,
  }));

  const rightPanelStyle = computed(() => ({
    flexBasis: `calc(${(1 - leftRatio.value) * 100}% - ${(1 - leftRatio.value) * splitterWidthPx}px)`,
    flexGrow: 0,
    flexShrink: 0,
  }));

  function clampLeftRatio(next: number): number {
    const containerWidth = containerRef.value?.clientWidth ?? 0;
    const availableWidth = containerWidth - splitterWidthPx;
    if (containerWidth <= 0 || availableWidth <= 0) return 0.5;
    const minRatio = Math.max(0.1, minLeftPx / availableWidth);
    const maxRatio = Math.min(0.9, 1 - minRightPx / availableWidth);
    if (minRatio > maxRatio) return 0.5;
    return Math.max(minRatio, Math.min(maxRatio, next));
  }

  function updateResize(clientX: number) {
    const rect = containerRef.value?.getBoundingClientRect();
    if (!rect) return;
    const splitterHalf = splitterWidthPx / 2;
    const minX = rect.left + splitterHalf;
    const maxX = rect.right - splitterHalf;
    const clampedX = Math.max(minX, Math.min(clientX, maxX));
    const availableWidth = rect.width - splitterWidthPx;
    if (availableWidth <= 0) return;
    const leftWidth = clampedX - rect.left - splitterHalf;
    const nextRatio = leftWidth / availableWidth;
    leftRatio.value = clampLeftRatio(nextRatio);
  }

  function handleResizeMove(event: MouseEvent) {
    updateResize(event.clientX);
  }

  function stopResize() {
    if (!isResizing.value) return;
    isResizing.value = false;
    window.removeEventListener("mousemove", handleResizeMove);
    window.removeEventListener("mouseup", stopResize);
    document.body.style.userSelect = "";
    document.body.style.cursor = "";
  }

  function startResize(event: MouseEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    isResizing.value = true;
    document.body.style.userSelect = "none";
    document.body.style.cursor = "col-resize";
    updateResize(event.clientX);
    window.addEventListener("mousemove", handleResizeMove);
    window.addEventListener("mouseup", stopResize);
  }

  onBeforeUnmount(() => {
    stopResize();
  });

  return {
    isResizing,
    leftRatio,
    leftPanelStyle,
    rightPanelStyle,
    startResize,
  };
}
