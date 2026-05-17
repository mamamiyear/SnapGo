<script setup lang="ts">
/*
 * Toast — bottom-right transient notification used for upload success /
 * failure messages. Self-managed lifecycle is owned by the parent App.vue,
 * which simply hides this component after a delay.
 */
defineProps<{
  kind: 'success' | 'error'
  text: string
}>()
</script>

<template>
  <div class="toast" :class="kind">{{ text }}</div>
</template>

<style scoped>
.toast {
  position: fixed;
  right: 18px;
  bottom: 18px;
  max-width: 380px;
  padding: 10px 14px;
  border-radius: 6px;
  font-size: 12.5px;
  color: #fff;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
  white-space: pre-wrap;
  word-break: break-all;
  animation: toast-in 0.18s ease-out;
}
.toast.success {
  background: #10b981;
}
.toast.error {
  background: #ef4444;
}
@keyframes toast-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
