<script setup lang="ts">
import { Button } from "@/components/ui/button";
import { confirmation, resolveConfirmation } from "@/lib/confirm";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
</script>
<template>
  <AlertDialog
    :open="!!confirmation"
    @update:open="
      (open) => {
        if (!open) resolveConfirmation(false);
      }
    "
  >
    <AlertDialogContent>
      <AlertDialogHeader
        ><AlertDialogTitle>确认操作</AlertDialogTitle
        ><AlertDialogDescription class="whitespace-pre-wrap break-words">{{
          confirmation?.message
        }}</AlertDialogDescription></AlertDialogHeader
      >
      <AlertDialogFooter
        ><AlertDialogCancel @click="resolveConfirmation(false)"
          >取消</AlertDialogCancel
        ><Button @click="resolveConfirmation(true)"
          >确认</Button
        ></AlertDialogFooter
      >
    </AlertDialogContent>
  </AlertDialog>
</template>
