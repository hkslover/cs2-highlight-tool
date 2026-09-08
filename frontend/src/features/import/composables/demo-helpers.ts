/** Compatibility exports for older import code. */
export {
  normalizeClipOverrides,
} from "@/domains/clip-selection";

export function normalizeSpecMode(_mode: unknown): 1 {
  return 1;
}

export function basename(path: string): string {
  const match = /[^\\/]+$/.exec(path);
  return match ? match[0] : path;
}
