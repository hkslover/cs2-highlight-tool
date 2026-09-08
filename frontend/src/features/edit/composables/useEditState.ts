import { useEditDomain } from "@/domains/edit";
export type {
  EditSequenceItem,
  EditTransitionMode,
} from "@/domains/edit";

/**
 * Compatibility facade for edit components. The state itself belongs to the
 * application edit domain so it survives route changes.
 */
export function useEditState() {
  return useEditDomain();
}
