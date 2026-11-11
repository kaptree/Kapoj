<script setup lang="ts">
import { computed } from 'vue'

import type { ActivityDay, UserActivity } from '@/api/types'

const CALENDAR_DAYS = 365

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
  gridTemplateColumns: `repeat(${calendar.value.weeks}, 11px)`,
}))

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
</script>

<template>
  <section class="contribution-grid" aria-labelledby="contribution-grid-title">
    <header class="contribution-grid__head">
      <h2 id="contribution-grid-title" class="contribution-grid__title">代码提交贡献图</h2>
      <p class="contribution-grid__summary">{{ summary }}</p>
    </header>

    <div class="contribution-scroll">
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

          <div class="contribution-cells" role="grid" :aria-label="gridLabel">
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
              role="gridcell"
              :title="cellLabel(day)"
              :aria-label="cellLabel(day)"
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
      <p class="contribution-grid__hint">每格代表一天，颜色越深表示当天代码提交越多。</p>
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
  --contrib-1: #0e4429;
  --contrib-2: #006d32;
  --contrib-3: #26a641;
  --contrib-4: #39d353;

  display: flex;
  flex-direction: column;
  gap: var(--kapoj-space-4);
  min-width: 0;
}

:global(:root:not(.app-dark)) .contribution-grid {
  --contrib-0: #e8eef5;
  --contrib-1: #b7f0c9;
  --contrib-2: #7ddba0;
  --contrib-3: #40c463;
  --contrib-4: #1a7f37;
}

.contribution-grid__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--kapoj-space-2) var(--kapoj-space-4);
}

.contribution-grid__title {
  margin: 0;
  color: var(--kapoj-text);
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.contribution-grid__summary {
  margin: var(--kapoj-space-1) 0 0;
  color: var(--kapoj-text-muted);
  font-size: 0.8125rem;
}

.contribution-scroll {
  padding-bottom: var(--kapoj-space-1);
  overflow-x: auto;
}

.contribution-calendar {
  display: flex;
  flex-direction: column;
  gap: 3px;
  width: max-content;
  min-width: 100%;
}

.contribution-months {
  display: grid;
  column-gap: 3px;
  margin-left: 26px;
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  color: var(--kapoj-text-muted);
}

.contribution-month {
  white-space: nowrap;
}

.contribution-body {
  display: flex;
  gap: 6px;
  align-items: flex-start;
}

.contribution-weekdays {
  display: grid;
  grid-template-rows: repeat(7, 11px);
  row-gap: 3px;
  width: 20px;
  color: var(--kapoj-text-muted);
  font-family: var(--kapoj-font-mono);
  font-size: 10px;
  line-height: 11px;
  text-align: right;
}

.contribution-weekday {
  white-space: nowrap;
}

.contribution-cells {
  display: grid;
  grid-template-rows: repeat(7, 11px);
  grid-auto-flow: column;
  grid-auto-columns: 11px;
  gap: 3px;
}

.contribution-cell {
  width: 11px;
  height: 11px;
  background: var(--contrib-0);
  border: 1px solid var(--kapoj-border);
  border-radius: 2px;
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
  margin: 0;
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
}

.contribution-legend {
  display: flex;
  align-items: center;
  gap: var(--kapoj-space-1);
  color: var(--kapoj-text-muted);
  font-size: 0.75rem;
}

.contribution-legend__swatch {
  width: 11px;
  height: 11px;
  background: var(--contrib-0);
  border: 1px solid var(--kapoj-border);
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
</style>
