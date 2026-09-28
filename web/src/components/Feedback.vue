<script setup lang="ts">
import { watch } from "vue";
import { toast } from "vue-sonner";
import { Alert, AlertDescription } from "@/components/ui/alert";
const props = defineProps<{ message: string; error: boolean }>();
watch(
  () => props.message,
  (message) => {
    if (message && !props.error) toast.success(message);
  },
);
</script>
<template>
  <Alert v-if="message && error" variant="destructive" class="mt-4"
    ><AlertDescription>{{ message }}</AlertDescription></Alert
  >
</template>
