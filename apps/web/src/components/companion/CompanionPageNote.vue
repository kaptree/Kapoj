<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import { pickCompanionLine } from '../../content/companion/lines.zh-CN'
import { COMPANION_PAGE_NOTES } from '../../content/companion/pageNotes'
import { useCompanionMute } from '../../composables/useCompanionMute'
import { useCompanionStore } from '../../stores/companion'
import { localDateKey } from '../../utils/date'
import CompanionNote from './CompanionNote.vue'

const route = useRoute()
const store = useCompanionStore()
const { muted } = useCompanionMute()

store.load()

const noteKey = computed(() => (typeof route.name === 'string' ? route.name : ''))
const note = computed(() => COMPANION_PAGE_NOTES[noteKey.value])
const line = computed(() =>
  note.value ? pickCompanionLine(note.value.lineKey, localDateKey()) : '',
)
const expression = computed(() => note.value?.expression ?? 'smug')
const visible = computed(
  () =>
    Boolean(note.value) &&
    store.enabled &&
    store.activityLevel !== 'quiet' &&
    !muted.value &&
    !store.isNoteDismissed(noteKey.value),
)

function dismiss(): void {
  store.dismissNote(noteKey.value)
}
</script>

<template>
  <CompanionNote v-if="visible" :line="line" :expression="expression" @dismiss="dismiss" />
</template>
