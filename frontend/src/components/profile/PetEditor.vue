<script setup lang="ts">
import type { Pet } from '@/types/api'

// PetList / PetEditor: add/edit/remove pets (breed, name, sex, castrated, age).
// Dumb component: edits the pets array in-place via v-model, emits add/remove.
defineProps<{ pets: Pet[] }>()
const emit = defineEmits<{ add: []; remove: [index: number] }>()
</script>

<template>
  <div class="card">
    <div style="display: flex; align-items: center">
      <h3 style="margin: 0">Pets</h3>
      <span style="flex: 1" />
      <button type="button" @click="emit('add')">+ Add pet</button>
    </div>

    <p v-if="pets.length === 0" style="color: var(--dm-muted)">
      No pets yet. Add your dog so friends know who you walk with.
    </p>

    <div
      v-for="(pet, i) in pets"
      :key="i"
      style="border-top: 1px solid var(--dm-border); padding-top: 0.75rem; margin-top: 0.75rem"
    >
      <div class="field">
        <label>Breed</label>
        <input v-model="pet.breed" />
      </div>
      <div class="field">
        <label>Name</label>
        <input v-model="pet.name" />
      </div>
      <div class="field">
        <label>Sex</label>
        <select v-model="pet.sex">
          <option value="M">Male</option>
          <option value="F">Female</option>
        </select>
      </div>
      <div class="field">
        <label>
          <input
            type="checkbox"
            v-model="pet.is_castrated"
            style="width: auto; margin-right: 0.4rem"
          />
          Castrated
        </label>
      </div>
      <div class="field">
        <label>Age (years)</label>
        <input v-model.number="pet.age" type="number" min="0" />
      </div>
      <button type="button" class="danger" @click="emit('remove', i)">
        Remove pet
      </button>
    </div>
  </div>
</template>
