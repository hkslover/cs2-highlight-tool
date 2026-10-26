import type { SelectOption } from "naive-ui";
import { t } from "@/shared/i18n";
import type { ComponentStatus, ProgressMessage } from "@/shared/types";

export type TagType = "default" | "success" | "info" | "warning" | "error";
export type MessageType = "default" | "success" | "info" | "warning" | "error";
export type TaskKind = "self_update" | "component";

export interface TaskItem {
  id: string;
  name: string;
  status: string;
  error: string;
  manual_url: string;
  kind: TaskKind;
  component?: ComponentStatus;
}

export function normalizeSelfUpdateStatus(
  status: string,
  available: boolean,
  running: boolean,
): string {
  switch (status) {
    case "checking":
    case "ready":
    case "failed":
    case "needs_action":
      return status;
    default:
      if (available) return "needs_action";
      return running ? "checking" : "pending";
  }
}

const idleProgress: ProgressMessage = {
  component_id: "",
  active: false,
  percent: 0,
  indeterminate: false,
};

export function progressFor(
  task: TaskItem,
  progressMap: Record<string, ProgressMessage>,
): ProgressMessage {
  return progressMap[task.id] || idleProgress;
}

export function showProgress(task: TaskItem): boolean {
  return ["checking", "downloading", "installing"].includes(task.status);
}

export function progressPercent(
  task: TaskItem,
  progressMap: Record<string, ProgressMessage>,
): number {
  const progress = progressFor(task, progressMap);
  if (!progress.active) return 0;
  return Math.max(0, Math.min(100, progress.percent || 0));
}

export function isIndeterminate(
  task: TaskItem,
  progressMap: Record<string, ProgressMessage>,
): boolean {
  const progress = progressFor(task, progressMap);
  if (!progress.active) return true;
  return progress.indeterminate;
}

export function showPercent(
  task: TaskItem,
  progressMap: Record<string, ProgressMessage>,
): boolean {
  return showProgress(task) && !isIndeterminate(task, progressMap);
}

export function statusText(status: string): string {
  const keyMap: Record<string, string> = {
    pending: "startup.status.pending",
    checking: "startup.status.checking",
    downloading: "startup.status.downloading",
    installing: "startup.status.installing",
    ready: "startup.status.ready",
    warning: "startup.status.warning",
    failed: "startup.status.failed",
    needs_action: "startup.status.needs_action",
  };
  const key = keyMap[status];
  return key ? t(key) : status;
}

export function statusTagType(status: string): TagType {
  switch (status) {
    case "ready":
      return "success";
    case "failed":
    case "needs_action":
      return "error";
    case "warning":
      return "warning";
    case "checking":
    case "downloading":
    case "installing":
      return "info";
    default:
      return "default";
  }
}

export function showReinstall(component: ComponentStatus, canEnterMain: boolean): boolean {
  return (
    canEnterMain &&
    ["hlae", "plugin", "ffmpeg"].includes(component.id) &&
    component.status === "ready"
  );
}

export function showActions(task: TaskItem, canEnterMain: boolean): boolean {
  if (task.kind === "self_update") {
    return task.status === "needs_action";
  }
  if (!task.component) {
    return false;
  }
  if (task.component.id === "cs2") {
    return true;
  }
  return (
    showReinstall(task.component, canEnterMain) ||
    task.status === "failed" ||
    task.status === "needs_action" ||
    task.status === "downloading" ||
    canRetry(task)
  );
}

export function canRetry(task: TaskItem): boolean {
  if (task.kind !== "component" || !task.component || task.component.id === "cs2") {
    return false;
  }
  if (task.status === "failed" || task.status === "needs_action" || task.status === "warning") {
    return true;
  }
  return task.status === "ready" && !!task.error;
}

export function taskMessageType(task: TaskItem): MessageType {
  if (task.status === "ready") return "default";
  if (task.status === "warning") return "warning";
  return "error";
}

export function taskMessageDepth(task: TaskItem): 1 | 2 | 3 {
  return task.status === "ready" ? 3 : 1;
}

export function componentName(componentID: string, fallback: string): string {
  const map: Record<string, string> = {
    hlae: "startup.components.hlae",
    plugin: "startup.components.plugin",
    ffmpeg: "startup.components.ffmpeg",
    cs2: "startup.components.cs2",
  };
  const key = map[componentID];
  return key ? t(key) : fallback || componentID;
}

export function versionWithPrefix(
  value: string,
  fallback = t("startup.version.unknown"),
): string {
  const raw = (value || "").trim();
  if (!raw) return fallback;
  const normalized = raw.replace(/^[vV]/, "");
  if (!normalized) return fallback;
  return `v${normalized}`;
}

export function taskVersionMeta(
  task: TaskItem,
  selfUpdate: { current: string; latest: string },
  component?: ComponentStatus,
): string {
  if (task.kind === "self_update") {
    return t("startup.version.self_update_meta", {
      current: versionWithPrefix(selfUpdate.current || "0.0.0"),
      latest: versionWithPrefix(selfUpdate.latest),
    });
  }
  if (!component) {
    return "";
  }
  if (component.id === "hlae" || component.id === "plugin") {
    return t("startup.version.component_meta", {
      local: versionWithPrefix(component.local_version),
      remote: versionWithPrefix(component.remote_version),
    });
  }
  return "";
}

export function showChannelSelect(component?: ComponentStatus): boolean {
  return component?.id === "hlae" && (component.channels?.length ?? 0) > 1;
}

export interface ChannelSelectOption extends SelectOption {
  label: string;
  value: string;
  note: string;
  installed: boolean;
}

export function channelSelectOptions(component: ComponentStatus): ChannelSelectOption[] {
  return (component.channels || []).map((channel) => ({
    label: `${channel.name || channel.id} · ${versionWithPrefix(channel.tag)}`,
    value: channel.id,
    note: (channel.note || "").trim(),
    installed: channel.installed,
  }));
}

export function canSelectChannel(task: TaskItem): boolean {
  return !["checking", "downloading", "installing"].includes(task.status);
}

export function importHint(componentID: string): string {
  switch (componentID) {
    case "hlae":
      return t("startup.import_hint.hlae");
    case "plugin":
      return t("startup.import_hint.plugin");
    case "ffmpeg":
      return t("startup.import_hint.ffmpeg");
    default:
      return t("startup.import_hint.default");
  }
}

export function cs2ActionText(status: string): string {
  if (status === "ready") {
    return t("startup.actions.reset_cs2");
  }
  return t("startup.actions.pick_cs2");
}
