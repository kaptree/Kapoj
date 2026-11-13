import { http } from './client'
import type { AdminMailView, MailSettingsInput, MailTestInput } from './types'

/** Admin mail settings view (mail.settings.manage). */
export const getAdminMail = () => http.get<AdminMailView>('/admin/mail')

/** Save mail settings; omitted smtpPassword keeps the stored password. */
export const updateAdminMail = (input: MailSettingsInput) =>
  http.put<AdminMailView>('/admin/mail', input)

/** Send a test email using the saved configuration. */
export const testAdminMail = (to: string) =>
  http.post<null>('/admin/mail/test', { to } satisfies MailTestInput)
