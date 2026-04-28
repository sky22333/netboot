<template>
  <div class="card p-5">
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold">系统诊断</h1>
      <button class="btn" :disabled="loading" @click="load">{{ loading ? '诊断中...' : '重新诊断' }}</button>
    </div>
    <p v-if="error" class="mt-3 text-sm text-red-600">{{ error }}</p>
    <pre class="mt-4 overflow-auto rounded-md bg-neutral-50 p-4 text-sm">{{ JSON.stringify(data, null, 2) }}</pre>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../lib/api'
const data = ref<any>()
const loading = ref(false)
const error = ref('')
async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api('/diagnostics')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '诊断失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
