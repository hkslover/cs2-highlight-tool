import { computed, h, type VNode } from "vue";
import { NTooltip, useDialog, useMessage, type SelectOption } from "naive-ui";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";
import type {
  ComponentStatus,
  ProgressMessage,
  StartupState,
  WorkspaceState,
} from "@/shared/types";
import {
  normalizeSelfUpdateStatus,
  showProgress,
  progressPercent,
  isIndeterminate,
  showPercent,
  statusText,
  statusTagType,
  showReinstall,
  showActions,
  canRetry,
  taskMessageType,
  taskMessageDepth,
  componentName,
  versionWithPrefix,
  taskVersionMeta,
  importHint,
  cs2ActionText,
  showChannelSelect,
  channelSelectOptions,
  canSelectChannel,
  type ChannelSelectOption,
  progressFor,
  type TagType,
  type MessageType,
  type TaskKind,
  type TaskItem,
} from "./startup-display";

export function useStartupWizard(props: {
  state: StartupState;
  progressMap: Record<string, ProgressMessage>;
}) {
  const message = useMessage();
  const dialog = useDialog();

  const busy = computed(() => props.state.running);

  const canReset = computed(() => !props.state.running);

  const tasks = computed<TaskItem[]>(() => {
    const selfUpdateStatus = normalizeSelfUpdateStatus(
      props.state.self_update.status,
      props.state.self_update.available,
      props.state.running,
    );

    const selfUpdateTask: TaskItem = {
      id: "self_update",
      name: t("startup.self_update_name"),
      status: selfUpdateStatus,
      error: props.state.self_update.error || "",
      manual_url: props.state.self_update.url || "",
      kind: "self_update",
    };

    const componentTasks = props.state.steps.map((step) => ({
      id: step.id,
      name: componentName(step.id, step.name),
      status: step.status,
      error: step.error || "",
      manual_url: step.manual_url || "",
      kind: "component" as const,
      component: step,
    }));

    return [selfUpdateTask, ...componentTasks];
  });

  function retry(componentID: string) {
    void backend.RetryStartupComponent(componentID);
  }

  function reinstall(componentID: string) {
    void backend.ReinstallStartupComponent(componentID);
  }

  // Same height as the 12px version text so the card height does not change.
  const channelSelectTheme = {
    peers: {
      InternalSelection: { heightTiny: "20px", fontSizeTiny: "12px" },
    },
  };

  // Downloaded channels are marked with a local-disk icon; a check mark would
  // read as the select's own "selected" mark.
  function renderChannelLabel(option: SelectOption): VNode {
    const channel = option as ChannelSelectOption;
    const children: VNode[] = [h("span", channel.label)];
    if (channel.installed) {
      children.push(
        h(
          "svg",
          {
            width: 13,
            height: 13,
            viewBox: "0 0 24 24",
            fill: "none",
            stroke: "currentColor",
            "stroke-width": 2,
            "stroke-linecap": "round",
            "stroke-linejoin": "round",
            role: "img",
            "aria-label": t("startup.channel.installed"),
            style: { color: "#85d3a7", flex: "0 0 auto" },
          },
          [
            h("path", { d: "M5.5 6h13L21 13v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-5z" }),
            h("path", { d: "M3 13h18" }),
            h("path", { d: "M17 16.5h.01" }),
          ],
        ),
      );
    }
    return h("span", { style: { alignItems: "center", display: "inline-flex", gap: "6px" } }, children);
  }

  // Note and download state are shown on hover rather than inline.
  function renderChannelOption({ node, option }: { node: VNode; option: SelectOption }): VNode {
    const channel = option as ChannelSelectOption;
    const lines = [channel.note, channel.installed ? t("startup.channel.installed") : ""].filter(Boolean);
    if (lines.length === 0) {
      return node;
    }
    return h(NTooltip, { placement: "left" }, { trigger: () => node, default: () => lines.join(" · ") });
  }

  async function selectChannel(channelID: string) {
    try {
      await backend.SelectHLAEChannel(channelID);
    } catch (err) {
      message.error(t("startup.toast.select_channel_failed", { error: String(err) }));
    }
  }

  function openManual(componentID: string) {
    void backend.OpenManualDownload(componentID);
  }

  function cancelDownload(componentID: string) {
    void backend.CancelStartupDownload(componentID);
  }

  function importManual(componentID: string) {
    void backend.ImportManualDownload(componentID);
  }

  function pickCS2Path() {
    void backend.PickCS2Path();
  }

  function openSelfUpdateDownload() {
    void backend.OpenManualDownload("self_update");
  }

  async function doEnterMain() {
    try {
      await backend.EnterMainApp();
    } catch (err) {
      message.error(t("startup.toast.enter_main_failed", { error: String(err) }));
    }
  }

  function enterMain() {
    const notice = (props.state.entry_notice || "").trim();
    if (notice) {
      dialog.warning({
        title: t("startup.dialog.enter_main_title"),
        content: notice,
        positiveText: t("startup.dialog.continue"),
        negativeText: t("startup.dialog.cancel"),
        onPositiveClick: () => {
          doEnterMain();
        },
      });
      return;
    }
    doEnterMain();
  }

  async function exportLogs() {
    try {
      const path = await backend.ExportStartupLogs();
      if (path && path.trim()) {
        message.success(t("startup.toast.logs_exported", { path }));
      }
    } catch (err) {
      message.error(t("startup.toast.export_logs_failed", { error: String(err) }));
    }
  }

  async function confirmReset() {
    let dataDir = "";
    try {
      const ws: WorkspaceState = await backend.GetWorkspaceState();
      dataDir = (ws?.data_dir ?? "").trim();
    } catch {
      dataDir = "";
    }

    dialog.warning({
      title: t("workspace.reset.confirm_title"),
      content: t("workspace.reset.confirm_content", { dataDir }),
      positiveText: t("workspace.reset.confirm_yes"),
      negativeText: t("workspace.reset.confirm_no"),
      positiveButtonProps: { type: "error" },
      onPositiveClick: async () => {
        try {
          await backend.ResetWorkspace();
          message.success(t("workspace.reset.success"));
        } catch (err) {
          message.error(t("workspace.reset.failure", { error: String(err) }));
        }
      },
    });
  }

  return {
    t,
    busy,
    canReset,
    tasks,
    statusText,
    statusTagType,
    showActions: (task: TaskItem) => showActions(task, props.state.can_enter_main),
    showReinstall: (component: ComponentStatus) => showReinstall(component, props.state.can_enter_main),
    canRetry,
    taskMessageType,
    taskMessageDepth,
    taskVersionMeta: (task: TaskItem) =>
      taskVersionMeta(task, props.state.self_update, task.component),
    showProgress,
    progressPercent: (task: TaskItem) => progressPercent(task, props.progressMap),
    isIndeterminate: (task: TaskItem) => isIndeterminate(task, props.progressMap),
    showPercent: (task: TaskItem) => showPercent(task, props.progressMap),
    importHint,
    cs2ActionText,
    showChannelSelect,
    channelSelectOptions,
    channelSelectTheme,
    renderChannelLabel,
    renderChannelOption,
    canSelectChannel,
    selectChannel,
    retry,
    reinstall,
    openManual,
    cancelDownload,
    importManual,
    pickCS2Path,
    openSelfUpdateDownload,
    enterMain,
    exportLogs,
    confirmReset,
  };
}
