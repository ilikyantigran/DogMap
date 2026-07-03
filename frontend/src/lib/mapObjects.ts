import type { ObjectType } from '@/types/api'

// Presentation metadata for the map-object types. Single source for labels and
// marker colors used by both the legend and the markers.
export const OBJECT_TYPE_META: Record<
  ObjectType,
  { label: string; color: string }
> = {
  PARK: { label: 'Park', color: '#2f7d4f' },
  DOG_PARK: { label: 'Dog park', color: '#b8860b' },
  DOG_BEACH: { label: 'Dog beach', color: '#2980b9' },
}

export const OBJECT_TYPES = Object.keys(OBJECT_TYPE_META) as ObjectType[]
