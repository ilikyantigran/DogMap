<script setup lang="ts">
import { OBJECT_TYPES, OBJECT_TYPE_META } from '@/lib/mapObjects'

// MapLegend: object types Park / Dog-park / Dog-beach with their marker colors.
</script>

<template>
  <div class="card" style="padding: 0.6rem 0.8rem">
    <strong style="font-size: 0.85rem">Legend</strong>
    <div style="display: flex; gap: 1rem; flex-wrap: wrap; margin-top: 0.4rem">
      <span
        v-for="t in OBJECT_TYPES"
        :key="t"
        style="display: inline-flex; align-items: center; gap: 0.35rem; font-size: 0.85rem"
      >
        <span
          :style="{
            width: '12px',
            height: '12px',
            borderRadius: '50%',
            background: OBJECT_TYPE_META[t].color,
            display: 'inline-block',
          }"
        />
        {{ OBJECT_TYPE_META[t].label }}
      </span>
    </div>
  </div>
</template>
