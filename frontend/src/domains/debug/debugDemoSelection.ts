import type { DemoListEntry } from "@/shared/types";

/**
 * Demo selection for the debug workbench. The debug page keeps its own key
 * instead of reusing the import/clip selection: changing what the debug page
 * will play must not change the import page (and clip production) selection,
 * and the import page keeps no control over the debug target. The state lives
 * here and is cleared together with the rest of the debug domain when the
 * workspace changes.
 */

/**
 * Resolve the Demo key the debug page should target.
 *
 * `currentKey` wins while it still exists, so an import-page change never
 * overrides a choice made on the debug page. `preferredKey`/`firstEntry` only
 * apply on the first initialization or after the current entry disappeared.
 */
export function resolveDebugDemoKey(
  entries: DemoListEntry[],
  currentKey: string | null,
  preferredKey: string | null,
): string | null {
  if (currentKey && entries.some((entry) => entry.key === currentKey)) {
    return currentKey;
  }
  if (preferredKey && entries.some((entry) => entry.key === preferredKey)) {
    return preferredKey;
  }
  return entries.length > 0 ? entries[0].key : null;
}

function parentDirectoryName(filePath: string): string {
  const segments = filePath.split(/[\\/]+/).filter((segment) => segment.length > 0);
  return segments.length >= 2 ? segments[segments.length - 2] : "";
}

/**
 * Dropdown options using the stable Demo key as the value. Imported files with
 * the same name in different match directories stay distinguishable, while the
 * common case keeps the plain file name; the full path stays visible in the
 * path row.
 */
export function buildDebugDemoOptions(
  entries: DemoListEntry[],
): { label: string; value: string }[] {
  const nameCounts = new Map<string, number>();
  for (const entry of entries) {
    nameCounts.set(entry.file_name, (nameCounts.get(entry.file_name) ?? 0) + 1);
  }
  return entries.map((entry) => {
    if ((nameCounts.get(entry.file_name) ?? 0) < 2) {
      return { label: entry.file_name, value: entry.key };
    }
    const parent = parentDirectoryName(entry.file_path);
    return {
      label: parent ? `${entry.file_name} · ${parent}` : entry.file_name,
      value: entry.key,
    };
  });
}
