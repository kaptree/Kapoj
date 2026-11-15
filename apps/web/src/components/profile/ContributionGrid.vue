<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import type { ActivityDay, UserActivity } from '@/api/types'

const CALENDAR_DAYS = 365
const CELL_GAP = 3
const LABEL_WIDTH = 20
const LABEL_GAP = 6

const props = defineProps<{
  activity: UserActivity
}>()

interface CalendarDay extends ActivityDay {
  level: number
}

interface MonthLabel {
  label: string
  column: number
}

const scroll = ref<HTMLElement | null>(null)
const cells = ref<HTMLElement | null>(null)
const cellSize = ref(11)
const hoveredDay = ref<CalendarDay | null>(null)
const activeDate = ref('')
let observer: ResizeObserver | undefined

function parseLocalDate(value: string): Date | null {
  if (!value) return null
  const date = new Date(`${value}T00:00:00`)
  return Number.isNaN(date.getTime()) ? null : date
}

function toDateKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function level(count: number): number {
  if (count <= 0) return 0
  if (count <= 2) return 1
  if (count <= 5) return 2
  if (count <= 9) return 3
  return 4
}

const calendar = computed(() => {
  const start = parseLocalDate(props.activity.startDate)
  const end = parseLocalDate(props.activity.endDate)
  const counts = new Map(props.activity.days.map((day) => [day.date, day.count]))

  const days: CalendarDay[] = []
  if (start && end && end >= start) {
    const cursor = new Date(start)
    for (let index = 0; index < CALENDAR_DAYS && cursor <= end; index += 1) {
      const date = toDateKey(cursor)
      const count = counts.get(date) ?? 0
      days.push({ date, count, level: level(count) })
      cursor.setDate(cursor.getDate() + 1)
    }
  } else {
    for (const day of props.activity.days.slice(0, CALENDAR_DAYS)) {
      days.push({ ...day, level: level(day.count) })
    }
  }

  const firstDay = days.length > 0 ? parseLocalDate(days[0].date) : null
  // Weeks start on Monday, matching the 一/三/五 weekday labels.
  const leading = firstDay ? (firstDay.getDay() + 6) % 7 : 0
  const weeks = Math.max(1, Math.ceil((leading + days.length) / 7))
  const trailing = Math.max(0, weeks * 7 - leading - days.length)

  return { days, leading, trailing, weeks }
})

watch(
  () => calendar.value.days,
  (days) => {
    if (days.length === 0) {
      activeDate.value = ''
      return
    }
    if (days.some((day) => day.date === activeDate.value)) return
    const latestActive = [...days].reverse().find((day) => day.count > 0)
    activeDate.value = latestActive?.date ?? days[days.length - 1].date
  },
  { immediate: true },
)

const monthLabels = computed<MonthLabel[]>(() => {
  const days = calendar.value.days
  if (days.length === 0) return []
  const firstDay = parseLocalDate(days[0].date)
  if (!firstDay) return []

  const weekStart = new Date(firstDay)
  weekStart.setDate(weekStart.getDate() - calendar.value.leading)

  const labels: MonthLabel[] = []
  let lastMonth = -1
  for (let week = 0; week < calendar.value.weeks; week += 1) {
    const cursor = new Date(weekStart)
    cursor.setDate(cursor.getDate() + week * 7)
    const month = cursor.getMonth()
    if (month !== lastMonth) {
      labels.push({ label: `${month + 1}月`, column: week + 1 })
      lastMonth = month
    }
  }
  return labels
})

const weekdays = [
  { row: 1, label: '一' },
  { row: 3, label: '三' },
  { row: 5, label: '五' },
]

const monthGridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${calendar.value.weeks}, var(--contrib-size))`,
}))

const gridStyle = computed<Record<string, string>>(() => ({
  '--contrib-size': `${cellSize.value}px`,
  '--contrib-gap': `${CELL_GAP}px`,
  '--contrib-label-width': `${LABEL_WIDTH}px`,
  '--contrib-label-gap': `${LABEL_GAP}px`,
}))

function updateCellSize() {
  const element = scroll.value
  if (!element) return
  const available = element.clientWidth - LABEL_WIDTH - LABEL_GAP
  if (available <= 0) return
  const raw = (available - (calendar.value.weeks - 1) * CELL_GAP) / calendar.value.weeks
  cellSize.value = Math.min(15, Math.max(9, Math.floor(raw * 10) / 10))
}

onMounted(() => {
  updateCellSize()
  if (typeof ResizeObserver !== 'undefined' && scroll.value) {
    observer = new ResizeObserver(updateCellSize)
    observer.observe(scroll.value)
  }
})

watch(
  () => calendar.value.weeks,
  () => updateCellSize(),
  { flush: 'post' },
)

onBeforeUnmount(() => observer?.disconnect())

const summary = computed(
  () =>
    `过去一年 ${props.activity.contributionTotal} 次代码提交 · 当前连续 ${props.activity.currentStreak} 天 · 最长连续 ${props.activity.longestStreak} 天`,
)

const gridLabel = computed(
  () =>
    `${props.activity.startDate} 至 ${props.activity.endDate} 的代码提交贡献图，共 ${props.activity.contributionTotal} 次代码提交`,
)

function cellLabel(day: ActivityDay): string {
  return `${day.date}：${day.count} 次代码提交`
}

function onCellFocus(day: CalendarDay) {
  activeDate.value = day.date
  hoveredDay.value = day
}

function moveCellFocus(event: KeyboardEvent, day: CalendarDay) {
  let target: CalendarDay | undefined

  if (event.key === 'Home') {
    target = calendar.value.days[0]
  } else if (event.key === 'End') {
    target = calendar.value.days[calendar.value.days.length - 1]
  } else {
    const offsets: Record<string, number> = {
      ArrowLeft: -1,
      ArrowRight: 1,
      ArrowUp: -7,
      ArrowDown: 7,
    }
    const offset = offsets[event.key]

    if (!offset) return
    const date = parseLocalDate(day.date)
    if (!date) return
    date.setDate(date.getDate() + offset)
    const targetDate = toDateKey(date)
    target = calendar.value.days.find((item) => item.date === targetDate)
  }

  if (!target) return
  event.preventDefault()
  activeDate.value = target.date
  hoveredDay.value = target
  void nextTick(() => {
    cells.value?.querySelector<HTMLElement>(`[data-date="${target!.date}"]`)?.focus()
  })
}
</script>

<template>
  <section class="contribution-grid" :style="gridStyle" aria-labelledby="contribution-grid-title">
    <header class="contribution-grid__head">
      <div class="contribution-grid__heading">
        <h2 id="contribution-grid-title" class="contribution-grid__title">代码提交贡献图</h2>
        <p class="contribution-grid__subtitle">最近 365 天的代码提交记录</p>
      </div>

      <dl class="contribution-grid__stats" aria-hidden="true">
        <div class="contribution-stat">
          <dt>年度提交</dt>
          <dd class="num">{{ activity.contributionTotal }}<span> 次</span></dd>
        </div>
        <div class="contribution-stat">
          <dt>当前连续</dt>
          <dd class="num">{{ activity.currentStreak }}<span> 天</span></dd>
        </div>
        <div class="contribution-stat">
          <dt>最长连续</dt>
          <dd class="num">{{ activity.longestStreak }}<span> 天</span></dd>
        </div>
      </dl>

      <p class="sr-only contribution-grid__summary">{{ summary }}</p>
    </header>

    <div ref="scroll" class="contribution-scroll">
      <div class="contribution-calendar">
        <div class="contribution-months" :style="monthGridStyle" aria-hidden="true">
          <span
            v-for="month in monthLabels"
            :key="`${month.label}-${month.column}`"
            class="contribution-month"
            :style="{ gridColumn: month.column }"
          >
            {{ month.label }}
          </span>
        </div>

        <div class="contribution-body">
          <div class="contribution-weekdays" aria-hidden="true">
            <span
              v-for="weekday in weekdays"
              :key="weekday.label"
              class="contribution-weekday"
              :style="{ gridRow: weekday.row }"
            >
              {{ weekday.label }}
            </span>
          </div>

          <div ref="cells" class="contribution-cells" role="group" :aria-label="gridLabel">
            <span
              v-for="blank in calendar.leading"
              :key="`leading-${blank}`"
              class="contribution-cell contribution-cell--blank"
              aria-hidden="true"
            />
            <span
              v-for="day in calendar.days"
              :key="day.date"
              class="contribution-cell"
              :class="`is-level-${day.level}`"
              role="img"
              :title="cellLabel(day)"
              :aria-label="cellLabel(day)"
              :data-date="day.date"
              :tabindex="day.date === activeDate ? 0 : -1"
              @mouseenter="hoveredDay = day"
              @mouseleave="hoveredDay = null"
              @focus="onCellFocus(day)"
              @blur="hoveredDay = null"
              @keydown="moveCellFocus($event, day)"
            />
            <span
              v-for="blank in calendar.trailing"
              :key="`trailing-${blank}`"
              class="contribution-cell contribution-cell--blank"
              aria-hidden="true"
            />
          </div>
        </div>
      </div>
    </div>

    <footer class="contribution-grid__footer">
      <p class="contribution-grid__hint" data-test="contribution-hover">
        <template v-if="hoveredDay">
          {{ hoveredDay.date }} · {{ hoveredDay.count }} 次代码提交
        </template>
        <template v-else>每格代表一天，颜色越深表示当天代码提交越多。</template>
      </p>
      <div class="contribution-legend" aria-label="贡献图图例">
        <span>少</span>
        <span
          v-for="swatch in 5"
          :key="swatch"
          class="contribution-legend__swatch"
          :class="`contribution-legend__swatch--${swatch - 1}`"
        />
        <span>多</span>
      </div>
    </footer>
  </section>
</template>

<style scoped>
.contribution-grid {
  --contrib-0: var(--kapoj-bg-elevated);
  /* Dark theme: fewer submissions use the lightest green, more use the
     deepest green. The light theme override below keeps the same direction. */
  --contrib-1: #39d353;
  --contrib-2: #26a641;
  --contrib-3: #006d32;
  --contrib-4: #0e4429;

  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

:root:not(.app-dark) .contribution-grid {
  --contrib-0: #edf2f7;
  --contrib-1: #b7f0c9;
  --contrib-2: #7ddba0;
  --contrib-3: #40c463;
  --contrib-4: #1a7f37;
}

.contribution-grid__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--kapoj-space-3) var(--kapoj-space-4);
}

.contribution-grid__heading {
  min-width: 0;
}

.contribution-grid__title {
  margin: 0;
  color: var(--profile-text, var(--kapoj-text));
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.contribution-grid__subtitle {
  margin: 4px 0 0;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 12px;
}

.contribution-grid__stats {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--kapoj-space-2) var(--kapoj-space-3);
  margin: 0;
}

.contribution-stat {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}

.contribution-stat dt {
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 10px;
  white-space: nowrap;
}

.contribution-stat dd {
  margin: 0;
  color: var(--profile-text, var(--kapoj-text));
  font-size: 16px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.03em;
}

.contribution-stat dd span {
  margin-left: 2px;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-sans);
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0;
}

.contribution-stat + .contribution-stat {
  padding-left: var(--kapoj-space-3);
  border-left: 1px solid var(--profile-border, var(--kapoj-border));
}

.contribution-scroll {
  padding-bottom: var(--kapoj-space-1);
  overflow-x: auto;
  scrollbar-width: thin;
}

.contribution-calendar {
  display: flex;
  flex-direction: column;
  gap: var(--contrib-gap);
  width: max-content;
  min-width: 100%;
}

.contribution-months {
  display: grid;
  column-gap: var(--contrib-gap);
  margin-left: calc(var(--contrib-label-width) + var(--contrib-label-gap));
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
}

.contribution-month {
  white-space: nowrap;
}

.contribution-body {
  display: flex;
  gap: var(--contrib-label-gap);
  align-items: flex-start;
}

.contribution-weekdays {
  display: grid;
  grid-template-rows: repeat(7, var(--contrib-size));
  row-gap: var(--contrib-gap);
  width: var(--contrib-label-width);
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  line-height: var(--contrib-size);
  text-align: right;
}

.contribution-weekday {
  white-space: nowrap;
}

.contribution-cells {
  display: grid;
  grid-template-rows: repeat(7, var(--contrib-size));
  grid-auto-flow: column;
  grid-auto-columns: var(--contrib-size);
  gap: var(--contrib-gap);
}

.contribution-cell {
  width: var(--contrib-size);
  height: var(--contrib-size);
  background: var(--contrib-0);
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 2px;
  transition:
    transform var(--kapoj-transition),
    box-shadow var(--kapoj-transition);
}

.contribution-cell:not(.contribution-cell--blank) {
  cursor: pointer;
}

.contribution-cell:not(.contribution-cell--blank):hover,
.contribution-cell:not(.contribution-cell--blank):focus-visible {
  z-index: 1;
  box-shadow: 0 0 0 2px
    color-mix(in srgb, var(--profile-primary, var(--kapoj-primary)) 55%, transparent);
  transform: scale(1.18);
}

.contribution-cell:not(.contribution-cell--blank):focus-visible {
  outline: 2px solid var(--profile-primary, var(--kapoj-primary));
  outline-offset: 2px;
}

.contribution-cell.is-level-1 {
  background: var(--contrib-1);
  border-color: transparent;
}

.contribution-cell.is-level-2 {
  background: var(--contrib-2);
  border-color: transparent;
}

.contribution-cell.is-level-3 {
  background: var(--contrib-3);
  border-color: transparent;
}

.contribution-cell.is-level-4 {
  background: var(--contrib-4);
  border-color: transparent;
}

.contribution-cell--blank {
  visibility: hidden;
}

.contribution-grid__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
}

.contribution-grid__hint {
  min-width: 0;
  margin: 0;
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
}

.contribution-legend {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
  color: var(--profile-text-muted, var(--kapoj-text-muted));
  font-size: 0.75rem;
}

.contribution-legend__swatch {
  width: var(--contrib-size);
  height: var(--contrib-size);
  background: var(--contrib-0);
  border: 1px solid var(--profile-border, var(--kapoj-border));
  border-radius: 2px;
}

.contribution-legend__swatch--1 {
  background: var(--contrib-1);
  border-color: transparent;
}

.contribution-legend__swatch--2 {
  background: var(--contrib-2);
  border-color: transparent;
}

.contribution-legend__swatch--3 {
  background: var(--contrib-3);
  border-color: transparent;
}

.contribution-legend__swatch--4 {
  background: var(--contrib-4);
  border-color: transparent;
}

@media (max-width: 640px) {
  .contribution-grid__stats {
    gap: var(--kapoj-space-1) var(--kapoj-space-3);
  }

  .contribution-stat + .contribution-stat {
    padding-left: var(--kapoj-space-2);
  }
}

@media (prefers-reduced-motion: reduce) {
  .contribution-cell {
    transition: none;
  }

  .contribution-cell:not(.contribution-cell--blank):hover,
  .contribution-cell:not(.contribution-cell--blank):focus-visible {
    transform: none;
  }
}
</style>
