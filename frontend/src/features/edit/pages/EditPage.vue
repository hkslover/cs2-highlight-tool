<template>
  <div class="edit-page">
    <n-card
      :bordered="true"
      class="edit-card"
      content-style="padding: 0;"
      content-class="edit-card-content"
    >
      <div class="page-head">
        <span class="panel-title">{{ t("main.edit.title") }}</span>
      </div>
      <div class="edit-body">
        <div ref="containerRef" class="edit-layout">
          <EditHistorySource :style="leftPanelStyle" />

          <div
            class="edit-splitter"
            :class="{ dragging: isResizing }"
            @mousedown="startResize"
          />

          <EditSequencePanel :style="rightPanelStyle" />
        </div>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { t } from "@/shared/i18n";
import EditHistorySource from "@/features/edit/components/EditHistorySource.vue";
import EditSequencePanel from "@/features/edit/components/EditSequencePanel.vue";
import { useSplitter } from "@/shared/composables/useSplitter";

const containerRef = ref<HTMLElement | null>(null);
const { isResizing, leftPanelStyle, rightPanelStyle, startResize } = useSplitter(containerRef, {
  initialLeftRatio: 0.44,
  minLeftPx: 280,
  minRightPx: 360,
  splitterWidthPx: 12,
});
</script>

<style scoped>
/*
 * 剪辑页整体滚动：卡片高度随内容增长，页面根节点作为唯一滚动容器，
 * 左右面板内部列表不再各自滚动，避免嵌套滚动条。
 */
.edit-page {
  height: 100%;
  overflow-y: auto;
  overflow-x: hidden;
}

.edit-card {
  min-height: 100%;
  background: #181b19;
}

.edit-card :deep(.edit-card-content) {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
}

.page-head {
  flex-shrink: 0;
  min-height: 34px;
  padding: 6px 10px;
  border-bottom: 1px solid #303732;
  display: flex;
  align-items: center;
}

.edit-body {
  flex: 1 1 auto;
  padding: 8px 10px 10px;
  display: flex;
  flex-direction: column;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.edit-layout {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
}

.edit-splitter {
  position: relative;
  flex: 0 0 12px;
  cursor: col-resize;
}

.edit-splitter::before {
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

.edit-splitter:hover::before,
.edit-splitter.dragging::before {
  background: #2f9462;
}

@media (max-width: 980px) {
  .edit-layout {
    flex-direction: column;
    gap: 10px;
  }

  .edit-splitter {
    display: none;
  }

  .edit-layout :deep(.source-panel),
  .edit-layout :deep(.sequence-panel) {
    flex: 1 1 auto !important;
  }
}
</style>
