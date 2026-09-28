import { onMounted, onUnmounted, shallowRef } from "vue";

export const pageRefresh = shallowRef<(() => Promise<void>) | null>(null);
export function usePageRefresh(refresh: () => Promise<void>) {
  onMounted(() => {
    pageRefresh.value = refresh;
  });
  onUnmounted(() => {
    if (pageRefresh.value === refresh) pageRefresh.value = null;
  });
}
