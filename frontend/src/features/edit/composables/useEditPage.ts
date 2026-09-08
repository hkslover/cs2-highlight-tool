import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import {
  initEditDomain,
  useEditDomain,
  type EditTransitionMode,
} from "@/domains/edit";

export function useEditPage() {
  const message = useMessage();
  const mounted = ref(false);
  try {
    // The app shell may initialize this earlier; this idempotent fallback
    // keeps the edit route usable while that integration is being adopted.
    initEditDomain();
  } catch {
    // Export can still use the backend while the Wails event bridge starts.
  }
  const domain = useEditDomain();

  onMounted(() => {
    mounted.value = true;
  });
  onBeforeUnmount(() => {
    mounted.value = false;
  });

  const composeProgressLabel = computed(() => {
    const step = String(domain.composeProgress.value.current_step || "").trim();
    return step || t("main.edit.exporting");
  });
  const exportError = computed(() => {
    const error = String(domain.exportError.value || "");
    return error ? t("main.edit.export_failed", { error }) : "";
  });

  const transitionDurationOptions = [
    { label: "0.3s", value: 0.3 },
    { label: "0.5s", value: 0.5 },
    { label: "1.0s", value: 1.0 },
  ];

  function handleTransitionModeChange(value: string | number) {
    const mode: EditTransitionMode = String(value) === "fade" ? "fade" : "none";
    domain.setTransitionMode(mode);
  }

  function handleTransitionDurationChange(value: string | number | null) {
    const next = Number(value);
    if (!Number.isFinite(next) || next <= 0) return;
    domain.setTransitionDuration(next);
  }

  function exportSequence() {
    // The promise is owned by the domain. A route change cannot cancel its
    // backend continuation or make completion depend on this component.
    return domain.exportSequence().then((result) => {
      if (result && mounted.value) {
        message.success(t("main.edit.export_success", { path: basename(result) }));
      }
      return result;
    });
  }

  async function openExportedClipFolder() {
    const target = String(domain.exportPath.value || "").trim();
    if (!target) return;
    try {
      await backend.OpenProducedClipInFolder(target);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      message.error(msg);
    }
  }

  return {
    sequenceItems: domain.sequenceItems,
    exporting: domain.exporting,
    exportError,
    exportPath: domain.exportPath,
    transitionMode: domain.transitionMode,
    transitionDuration: domain.transitionDuration,
    totalDuration: domain.totalDuration,
    composeProgress: domain.composeProgress,
    composePercent: domain.composePercent,
    composeProgressLabel,
    transitionDurationOptions,
    handleTransitionModeChange,
    handleTransitionDurationChange,
    exportSequence,
    clearExportError: domain.clearExportError,
    openExportedClipFolder,
  };
}

function basename(path: string): string {
  if (!path) return "";
  return path.replaceAll("\\", "/").split("/").pop() || path;
}
