import { shallowRef } from "vue";

export const confirmation = shallowRef<{
  message: string;
  resolve: (accepted: boolean) => void;
} | null>(null);

export function confirmAction(message: string): Promise<boolean> {
  if (confirmation.value) return Promise.resolve(false);
  return new Promise((resolve) => {
    confirmation.value = { message, resolve };
  });
}

export function resolveConfirmation(accepted: boolean) {
  const pending = confirmation.value;
  confirmation.value = null;
  pending?.resolve(accepted);
}
