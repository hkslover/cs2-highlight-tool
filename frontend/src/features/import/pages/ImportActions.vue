<template>
  <div class="import-actions">
    <n-card
      :bordered="true"
      class="actions-card"
      content-style="height: 100%; display: flex; flex-direction: column; padding: 0;"
      content-class="actions-card-content"
    >
      <div class="panel-head">
        <span class="panel-title">{{ t("main.import.actions_title") }}</span>
      </div>
      <div class="method-grid">
        <button
          type="button"
          class="method-card"
          :disabled="importing"
          @click="handleFileImport"
        >
          <div class="method-card-icon">
            <n-spin v-if="importing" size="small" />
            <n-icon v-else size="20"><FileIcon /></n-icon>
          </div>
          <div class="method-card-title">{{ t("main.import.btn_file") }}</div>
          <div class="method-card-desc">{{ t("main.import.card_file_desc") }}</div>
        </button>

        <button
          type="button"
          class="method-card"
          :disabled="importing"
          @click="$router.push({ name: 'import-wanmei' })"
        >
          <div class="method-card-icon">
            <img :src="wanmeiIconURL" alt="" class="wanmei-icon" />
          </div>
          <div class="method-card-title">{{ t("main.import.btn_wanmei") }}</div>
          <div class="method-card-desc">{{ t("main.import.card_wanmei_desc") }}</div>
        </button>

        <button
          type="button"
          class="method-card"
          :disabled="importing"
          @click="$router.push({ name: 'import-5e' })"
        >
          <div class="method-card-icon">
            <img :src="fiveeIconURL" alt="" class="wanmei-icon" />
          </div>
          <div class="method-card-title">{{ t("main.import.btn_5e") }}</div>
          <div class="method-card-desc">{{ t("main.import.card_5e_desc") }}</div>
        </button>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { h, ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import wanmeiIconURL from "@/assets/images/platforms/wanmei.ico";
import fiveeIconURL from "@/assets/images/platforms/fivee.ico";

const emit = defineEmits<{
  (e: "demos-selected", paths: string[]): void;
}>();

const importing = ref(false);
const message = useMessage();

async function handleFileImport() {
  try {
    importing.value = true;
    const paths = await backend.PickDemoFiles();
    if (!paths || paths.length === 0) return;
    emit("demos-selected", paths);
  } catch (err: any) {
    message.error(err?.message || String(err));
  } finally {
    importing.value = false;
  }
}

// Placeholder SVG icons — will be replaced later
const FileIcon = {
  render: () =>
    h(
      "svg",
      { xmlns: "http://www.w3.org/2000/svg", viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": "2", "stroke-linecap": "round", "stroke-linejoin": "round" },
      [
        h("path", { d: "M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" }),
        h("polyline", { points: "14 2 14 8 20 8" }),
      ],
    ),
};

</script>

<style scoped>
.import-actions {
  height: 100%;
}

.actions-card {
  height: 100%;
  background: #181b19;
  display: flex;
  flex-direction: column;
}

.actions-card :deep(.actions-card-content) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-head {
  display: flex;
  align-items: center;
  min-height: 34px;
  padding: 6px 10px;
  border-bottom: 1px solid #303732;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
}

.method-grid {
  flex: 1;
  min-height: 0;
  padding: 8px 10px 10px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  align-content: start;
  overflow: auto;
}

.method-card {
  border: 1px solid #303732;
  border-radius: 12px;
  background: linear-gradient(180deg, #1b1f1d 0%, #171a18 100%);
  color: #edf1ee;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
  min-height: 118px;
  padding: 12px 12px 10px;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.2s ease, transform 0.15s ease, background-color 0.2s ease;
}

.method-card:hover:not(:disabled) {
  border-color: #2f9462;
  transform: translateY(-1px);
}

.method-card:focus-visible {
  outline: 2px solid #2f9462;
  outline-offset: 1px;
}

.method-card:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.method-card-icon {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: 1px solid #2f3631;
  background: rgba(47, 148, 98, 0.1);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
}

.method-card-title {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.25;
}

.method-card-desc {
  margin-top: 5px;
  font-size: 12px;
  color: #97a49c;
  line-height: 1.45;
}

.wanmei-icon {
  width: 20px;
  height: 20px;
}

@media (max-width: 1080px) {
  .method-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .method-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
