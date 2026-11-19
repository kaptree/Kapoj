import { http } from './client'
import type {
  CompanionDialogueRequest,
  CompanionDialogueResult,
  CompanionEvent,
  CompanionPetResult,
  CompanionSettingsPatch,
  CompanionState,
  UserCompanion,
} from './types'

/** Current user's companion bond, level, settings and today's events. */
export const getCompanion = () => http.get<CompanionState>('/companion')

/** Unread companion events, oldest first. The backend caps `limit` at 20. */
export async function listCompanionEvents(limit?: number): Promise<CompanionEvent[]> {
  const data = await http.get<{ items: CompanionEvent[] }>(
    '/companion/events',
    limit === undefined ? undefined : { params: { limit } },
  )
  return Array.isArray(data?.items) ? data.items : []
}

/** Marks events as read; unknown or already-read ids are ignored by the server. */
export const markCompanionEventsRead = (ids: number[]) =>
  http.post<void>('/companion/events/read', { ids })

/** Partial settings update; only the fields present in `input` are changed. */
export const updateCompanionSettings = (input: CompanionSettingsPatch) =>
  http.post<CompanionState>('/companion/settings', input)

/** Pets the companion once per local day; repeat calls are idempotent. */
export const petCompanion = () => http.post<CompanionPetResult>('/companion/pet')

/** Public companion summary for a user's profile; private profiles return 404. */
export const getUserCompanion = (userID: number) =>
  http.get<UserCompanion>(`/users/${userID}/companion`)

/** Generate one AI companion line; the server falls back to a static lineKey. */
export const requestCompanionDialogue = (input: CompanionDialogueRequest, signal?: AbortSignal) =>
  http.post<CompanionDialogueResult>('/companion/dialogue', input, { signal })
