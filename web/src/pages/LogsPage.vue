<template>
  <Card class="p-6">
    <div
      class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
    >
      <div>
        <h1 class="text-lg font-semibold">运行日志</h1>
        <p class="text-sm text-muted-foreground">
          最新日志显示在底部，查看历史时暂停跟随。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <Button variant="outline" @click="toggleFollow">{{
          follow ? "暂停跟随" : "回到最新"
        }}</Button>
        <Button variant="outline" :disabled="loading" @click="refresh">{{
          loading ? "同步中..." : "同步历史"
        }}</Button>
      </div>
    </div>
    <p v-if="error" class="mt-3 text-sm text-red-600">{{ error }}</p>
    <div class="mt-4 overflow-hidden rounded-md border bg-background">
      <div
        class="flex flex-col gap-1 border-b px-3 py-2 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
      >
        <span
          >{{ connected ? "实时连接正常" : "实时连接重试中" }}，最多保留最近
          1000 条。</span
        >
        <span>{{ follow ? "自动跟随最新日志" : "已暂停，点击回到最新" }}</span>
      </div>
      <div ref="eventBox" class="max-h-[72vh] overflow-auto" @scroll="onScroll">
        <div
          v-for="line in events"
          :key="line.id"
          class="grid gap-1 border-b px-3 py-2 text-xs sm:grid-cols-[6rem_5.5rem_1fr] sm:gap-3"
        >
          <span class="text-muted-foreground">{{ shortTime(line.time) }}</span>
          <span :class="levelClass(line.level)">{{ line.source }}</span>
          <span class="min-w-0 break-words text-neutral-800">{{
            line.message
          }}</span>
        </div>
        <div
          v-if="events.length === 0"
          class="p-6 text-sm text-muted-foreground"
        >
          暂无日志，服务运行后会显示在这里。
        </div>
      </div>
    </div>
  </Card>
</template>

<script setup lang="ts">
import { usePageRefresh } from "@/lib/pageRefresh";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { nextTick, onMounted, ref, watch } from "vue";
import { useEventLog } from "../lib/eventLog";

const { events, connected, loading, error, load } = useEventLog();
const follow = ref(true);
const eventBox = ref<HTMLElement>();

async function refresh() {
  await load(800);
  await scrollToBottom();
}
function toggleFollow() {
  follow.value = !follow.value;
  if (follow.value) scrollToBottom();
}
function onScroll() {
  const box = eventBox.value;
  if (!box) return;
  const distance = box.scrollHeight - box.scrollTop - box.clientHeight;
  follow.value = distance < 40;
}
async function scrollToBottom() {
  await nextTick();
  if (eventBox.value) eventBox.value.scrollTop = eventBox.value.scrollHeight;
}
function levelClass(level: string) {
  if (level === "error") return "text-red-600 font-medium";
  if (level === "warning") return "text-amber-600 font-medium";
  return "text-neutral-700 font-medium";
}
function shortTime(value: string) {
  if (!value) return "";
  const match = value.match(/T(\d{2}:\d{2}:\d{2})/);
  return match?.[1] ?? value.slice(0, 19);
}

watch(
  () => events.value.at(-1)?.id,
  () => {
    if (follow.value) scrollToBottom();
  },
);

onMounted(async () => {
  await load(800);
  await scrollToBottom();
});
usePageRefresh(refresh);
</script>
