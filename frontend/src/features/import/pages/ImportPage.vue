<template>
  <div class="import-page" :class="{ 'detail-expanded': detailExpanded }">
    <div
      ref="upperRef"
      class="import-upper"
    >
      <div
        class="import-list-panel"
        :style="listPanelStyle"
      >
        <ImportDemoList
          :demo-list="demoList"
          :selected-index="selectedIndex"
          :format-duration="formatDuration"
          @select="toggleSelected"
          @remove="removeDemoAt"
        />
      </div>

      <div
        class="import-splitter"
        :class="{ dragging: isResizing }"
        @mousedown="startResize"
      />

      <div
        class="import-action-panel"
        :style="actionPanelStyle"
      >
        <router-view @demos-selected="onDemosSelected" />
      </div>
    </div>

    <div v-if="selectedEntry" class="import-detail-panel">
      <ImportDetailPanel
        :detail-collapsed="detailCollapsed"
        :selected-entry="selectedEntry"
        :selected-demo="selectedDemo"
        :can-select-prev="canSelectPrev"
        :can-select-next="canSelectNext"
        :format-duration="formatDuration"
        @select-prev="selectPrevDemo"
        @select-next="selectNextDemo"
        @toggle-collapse="toggleDetailCollapsed"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import ImportDemoList from "@/features/import/components/ImportDemoList.vue";
import ImportDetailPanel from "@/features/import/components/ImportDetailPanel.vue";
import { useImportDemos } from "@/features/import/composables/useImportDemos";
import { useSplitter } from "@/shared/composables/useSplitter";

const {
  demoList,
  selectedIndex,
  detailCollapsed,
  selectedEntry,
  selectedDemo,
  canSelectPrev,
  canSelectNext,
  onDemosSelected,
  removeDemoAt,
  toggleSelected,
  formatDuration,
  selectPrevDemo,
  selectNextDemo,
  toggleDetailCollapsed,
} = useImportDemos();

const upperRef = ref<HTMLElement | null>(null);
const {
  isResizing,
  leftPanelStyle: listPanelStyle,
  rightPanelStyle: actionPanelStyle,
  startResize,
} = useSplitter(upperRef, {
  initialLeftRatio: 0.3,
  minLeftPx: 260,
  minRightPx: 260,
  splitterWidthPx: 12,
});

const detailExpanded = computed(() => !detailCollapsed.value && !!selectedEntry.value);
</script>

<style scoped>
.import-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 10px;
  min-height: 0;
}

.import-upper {
  display: flex;
  flex: 1;
  min-height: 0;
}

.import-list-panel {
  flex: 0 0 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.import-action-panel {
  flex: 0 0 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.import-splitter {
  position: relative;
  flex: 0 0 12px;
  cursor: col-resize;
}

.import-splitter::before {
  content: "";
  position: absolute;
  left: 50%;
  top: 8px;
  bottom: 8px;
  width: 2px;
  border-radius: 999px;
  background: #303732;
  transform: translateX(-50%);
  transition: background-color 0.2s ease;
}

.import-splitter:hover::before,
.import-splitter.dragging::before {
  background: #2f9462;
}

.import-detail-panel {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

/* When the detail is expanded, hide the upper area and let the detail panel
   fill the entire view. The flex chain below guarantees the inner scroll
   container actually scrolls. */
.import-page.detail-expanded .import-upper {
  display: none;
}

.import-page.detail-expanded {
  overflow: hidden;
}

.import-page.detail-expanded .import-detail-panel {
  flex: 1 1 0;
  min-height: 0;
  position: relative;
}

.import-page.detail-expanded :deep(.detail-panel) {
  position: absolute;
  inset: 0;
}
</style>
