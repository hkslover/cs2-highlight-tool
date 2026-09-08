import { computed, ref } from "vue";
import { t } from "@/shared/i18n";
import { backend } from "@/shared/backend";

const path = ref("");
const errorMessage = ref("");
const validated = ref(false);
const submitting = ref(false);

export function useWorkspaceInit() {
  const canSubmit = computed(
    () => !submitting.value && path.value.trim().length > 0 && validated.value && !errorMessage.value,
  );

  function reset(): void {
    path.value = "";
    errorMessage.value = "";
    validated.value = false;
    submitting.value = false;
  }

  async function validate(): Promise<void> {
    const trimmed = path.value.trim();
    if (!trimmed) {
      errorMessage.value = "";
      validated.value = false;
      return;
    }
    try {
      const result = await backend.ValidateWorkspaceDir(trimmed);
      validated.value = result.ok;
      errorMessage.value = result.ok
        ? ""
        : result.errorMessage || t("workspace.validate.generic");
    } catch (err) {
      validated.value = false;
      errorMessage.value = String(err);
    }
  }

  async function pick(): Promise<void> {
    try {
      const selected = (await backend.PickWorkspaceDir()) || "";
      const trimmed = String(selected).trim();
      if (!trimmed) return;
      path.value = trimmed;
      await validate();
    } catch (err) {
      errorMessage.value = String(err);
      validated.value = false;
    }
  }

  async function confirm(): Promise<boolean> {
    if (!canSubmit.value) return false;
    submitting.value = true;
    try {
      await backend.SetWorkspaceDir(path.value.trim());
      // Reset on success so a later reset → modal reopen starts clean.
      reset();
      return true;
    } catch (err) {
      errorMessage.value = String(err);
      validated.value = false;
      return false;
    } finally {
      submitting.value = false;
    }
  }

  async function exitApp(): Promise<void> {
    try {
      await backend.ExitApp();
    } catch {
      // Ignore: process is supposed to terminate.
    }
  }

  return {
    t,
    path,
    errorMessage,
    submitting,
    canSubmit,
    pick,
    validate,
    confirm,
    exitApp,
    reset,
  };
}
