<template>
  <n-card size="small" :bordered="true" class="section-card">
    <template #header>
      <span class="section-title">{{ title }}</span>
    </template>

    <n-space vertical :size="12">
      <div class="storage-summary">
        <div class="storage-stat">
          <span class="storage-stat-value">{{ primaryValue }}</span>
          <span class="storage-stat-label">{{ primaryLabel }}</span>
        </div>
        <div class="storage-stat">
          <span class="storage-stat-value">{{ totalSize }}</span>
          <span class="storage-stat-label">{{ totalSizeLabel }}</span>
        </div>
      </div>
      <div class="storage-dir-row">
        <span class="setting-label">{{ pathLabel }}</span>
        <span class="storage-dir-path" :title="path">{{ path || "-" }}</span>
      </div>
      <div class="storage-actions">
        <n-button size="small" :loading="loading" @click="emitRefresh">
          {{ refreshLabel }}
        </n-button>
        <n-button size="small" :loading="opening" @click="emitOpen">
          {{ openLabel }}
        </n-button>
        <n-button size="small" type="error" :loading="clearing" :disabled="clearDisabled" @click="emitClear">
          {{ clearLabel }}
        </n-button>
      </div>
    </n-space>
  </n-card>
</template>

<script setup lang="ts">
defineProps<{
  title: string;
  primaryValue: number;
  primaryLabel: string;
  totalSize: string;
  totalSizeLabel: string;
  pathLabel: string;
  path: string;
  refreshLabel: string;
  openLabel: string;
  clearLabel: string;
  loading: boolean;
  opening: boolean;
  clearing: boolean;
  clearDisabled?: boolean;
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
  (e: "open"): void;
  (e: "clear"): void;
}>();

function emitRefresh(): void {
  emit("refresh");
}

function emitOpen(): void {
  emit("open");
}

function emitClear(): void {
  emit("clear");
}
</script>

<style scoped>
.section-card {
  background: #1a1e1b;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
}

.setting-label {
  color: #c9d3cb;
  font-size: 13px;
}

.storage-summary {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.storage-stat {
  background: #141715;
  border: 1px solid #303732;
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
}

.storage-stat-value {
  color: #edf1ee;
  font-size: 18px;
  font-weight: 700;
}

.storage-stat-label {
  color: #8d9890;
  font-size: 12px;
}

.storage-dir-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.storage-dir-path {
  color: #8d9890;
  font-size: 12px;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.storage-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
