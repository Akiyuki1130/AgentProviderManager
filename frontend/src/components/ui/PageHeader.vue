<template>
  <header class="page-header">
    <div class="ph-main">
      <span v-if="$slots.icon" class="ph-icon"><slot name="icon" /></span>
      <div class="ph-text">
        <h1 class="ph-title">{{ title }}</h1>
        <p v-if="subtitle" class="ph-sub">{{ subtitle }}</p>
      </div>
    </div>
    <div v-if="$slots.actions" class="ph-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  padding: 2px 2px 10px;
}
.ph-main { display: flex; align-items: center; gap: 10px; min-width: 0; }
.ph-icon { display: flex; align-items: center; color: var(--accent-text-strong, var(--accent)); flex-shrink: 0; }
.ph-text { min-width: 0; }
.ph-title { margin: 0; font-size: 18px; font-weight: 600; line-height: 1.3; letter-spacing: 0.1px; }
.ph-sub { margin: 2px 0 0; font-size: 12px; color: var(--fluent-text-soft); }
.ph-actions { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
</style>
