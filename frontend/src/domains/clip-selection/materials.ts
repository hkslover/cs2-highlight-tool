import type {
  ClipParameterOverrides,
  ClipPrimaryView,
  DemoClipKill,
  DemoListEntry,
  DemoMaterialSelection,
} from "@/shared/types";
import {
  getDemoMaterials,
  setDemoMaterials,
} from "./selection-state";

export function normalizeClipOverrides(
  input: Partial<ClipParameterOverrides> | undefined,
): ClipParameterOverrides | undefined {
  if (!input) return undefined;
  const next: ClipParameterOverrides = {};
  if (typeof input.killer_pre_seconds === "number" && Number.isFinite(input.killer_pre_seconds)) {
    next.killer_pre_seconds = input.killer_pre_seconds;
  }
  if (typeof input.killer_post_seconds === "number" && Number.isFinite(input.killer_post_seconds)) {
    next.killer_post_seconds = input.killer_post_seconds;
  }
  if (typeof input.victim_pre_seconds === "number" && Number.isFinite(input.victim_pre_seconds)) {
    next.victim_pre_seconds = input.victim_pre_seconds;
  }
  if (typeof input.victim_post_seconds === "number" && Number.isFinite(input.victim_post_seconds)) {
    next.victim_post_seconds = input.victim_post_seconds;
  }
  if (typeof input.enable_voice === "boolean") next.enable_voice = input.enable_voice;
  if (typeof input.enable_spec_show_xray_zero === "boolean") {
    next.enable_spec_show_xray_zero = input.enable_spec_show_xray_zero;
  }
  return Object.keys(next).length > 0 ? next : undefined;
}

export function addMaterialSelection(
  entry: DemoListEntry | null,
  kill: DemoClipKill,
  includeVictim: boolean,
  includeKiller = true,
  primaryView: ClipPrimaryView = "killer",
): void {
  if (!entry || !kill.id) return;
  const current = getDemoMaterials(entry);
  const existingIndex = current.findIndex((item) => item.kill.id === kill.id);
  if (existingIndex >= 0) {
    const next = current.slice();
    next[existingIndex] = {
      ...next[existingIndex],
      include_victim: next[existingIndex].include_victim || includeVictim,
      include_killer: next[existingIndex].include_killer !== false || includeKiller,
      killer_spec_mode: 1,
      victim_spec_mode: 1,
    };
    setDemoMaterials(entry, next);
    return;
  }
  setDemoMaterials(entry, current.concat({
    kill,
    include_killer: includeKiller,
    include_victim: includeVictim,
    killer_spec_mode: 1,
    victim_spec_mode: 1,
    primary_view: primaryView,
  }));
}

export function updateMaterialSpecModes(
  entry: DemoListEntry | null,
  killID: string,
  _patch: Partial<Pick<DemoMaterialSelection, "killer_spec_mode" | "victim_spec_mode">>,
): void {
  updateMaterial(entry, killID, (item) => ({
    ...item,
    killer_spec_mode: 1,
    victim_spec_mode: 1,
  }));
}

export function updateMaterialClipOverrides(
  entry: DemoListEntry | null,
  killID: string,
  patch: Partial<ClipParameterOverrides>,
): void {
  updateMaterial(entry, killID, (item) => ({
    ...item,
    clip_overrides: normalizeClipOverrides({ ...(item.clip_overrides ?? {}), ...patch }),
  }));
}

export function updateMaterialIncludeVictim(
  entry: DemoListEntry | null,
  killID: string,
  includeVictim: boolean,
): void {
  updateMaterial(entry, killID, (item) => ({ ...item, include_victim: includeVictim }));
}

export function updateMaterialIncludeKiller(
  entry: DemoListEntry | null,
  killID: string,
  includeKiller: boolean,
): void {
  updateMaterial(entry, killID, (item) => ({ ...item, include_killer: includeKiller }));
}

export function removeMaterialSelection(entry: DemoListEntry | null, killID: string): void {
  if (!entry || !killID) return;
  setDemoMaterials(entry, getDemoMaterials(entry).filter((item) => item.kill.id !== killID));
}

export function isKillSelectedInDemo(entry: DemoListEntry | null, killID: string): boolean {
  if (!entry || !killID) return false;
  return getDemoMaterials(entry).some((item) => item.kill.id === killID);
}

export function getMaterialSelections(entry: DemoListEntry | null): DemoMaterialSelection[] {
  return getDemoMaterials(entry);
}

export function getMaterialSelectionCount(entry: DemoListEntry | null): number {
  return getDemoMaterials(entry).length;
}

export function clearMaterialSelections(entry: DemoListEntry | null): void {
  if (entry) setDemoMaterials(entry, []);
}

function updateMaterial(
  entry: DemoListEntry | null,
  killID: string,
  update: (item: DemoMaterialSelection) => DemoMaterialSelection,
): void {
  if (!entry || !killID) return;
  const current = getDemoMaterials(entry);
  const index = current.findIndex((item) => item.kill.id === killID);
  if (index < 0) return;
  const next = current.slice();
  next[index] = update(next[index]);
  setDemoMaterials(entry, next);
}
