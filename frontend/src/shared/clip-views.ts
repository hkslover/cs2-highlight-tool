import type { ClipPrimaryView, DemoClipKill, DemoMaterialSelection } from "@/shared/types";

/**
 * A recording pass is bound to a role in the kill event — the killer pass
 * always specs the killer, the victim pass always specs the victim. What the
 * clip page shows instead is the pass's *position* relative to the player the
 * user picked in the selector:
 *
 *   - "primary"  — that player's own first-person view (主视角)
 *   - "opponent" — the other side of the same kill (对方视角)
 *
 * An item's primary_view says which role the selected player played, so it is
 * what maps positions back to roles. The same swap is applied to the default
 * recording windows in internal/app/plugin_generate.go.
 */
export type ViewPosition = "primary" | "opponent";
export type WindowEdge = "pre" | "post";

export function primaryViewOf(item: DemoMaterialSelection): ClipPrimaryView {
  return item.primary_view === "victim" ? "victim" : "killer";
}

export function roleOfPosition(item: DemoMaterialSelection, position: ViewPosition): ClipPrimaryView {
  const primary = primaryViewOf(item);
  if (position === "primary") return primary;
  return primary === "killer" ? "victim" : "killer";
}

export function isRoleIncluded(item: DemoMaterialSelection, role: ClipPrimaryView): boolean {
  return role === "killer" ? item.include_killer !== false : !!item.include_victim;
}

export function isPrimaryIncluded(item: DemoMaterialSelection): boolean {
  return isRoleIncluded(item, roleOfPosition(item, "primary"));
}

export function isOpponentIncluded(item: DemoMaterialSelection): boolean {
  return isRoleIncluded(item, roleOfPosition(item, "opponent"));
}

/** A suicide has no second camera to record — both sides are the same player. */
export function isSelfKill(kill: DemoClipKill): boolean {
  return !!kill.killer_steam_id && kill.killer_steam_id === kill.victim_steam_id;
}
