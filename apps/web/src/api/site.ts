import { http } from './client'
import type { SiteAdminView, SiteFooterInput, SiteHomeContent, SitePublicView } from './types'

/** Public site configuration: branding, home content, footer, uptime and visits. */
export const getSite = () => http.get<SitePublicView>('/site')

/** Increment the visit counter and return the latest count (public, no login). */
export const recordSiteVisit = () => http.post<{ visitCount: number }>('/site/visit')

/** Admin read model: same shape as the public view (site.settings.manage). */
export const getAdminSite = () => http.get<SiteAdminView>('/admin/site')

/** Replace the whole home content; the body is SiteHomeContent itself (site.settings.manage). */
export const updateAdminSiteHome = (input: SiteHomeContent) =>
  http.put<SiteAdminView>('/admin/site/home', input)

/** Update the footer fields and running start date (site.settings.manage). */
export const updateAdminSiteFooter = (input: SiteFooterInput) =>
  http.put<SiteAdminView>('/admin/site/footer', input)

/** Upload or replace the site logo as multipart/form-data (site.settings.manage). */
export const uploadAdminSiteLogo = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<SiteAdminView>('/admin/site/logo', form)
}

/** Remove the site logo; idempotent when absent (site.settings.manage). */
export const deleteAdminSiteLogo = () => http.delete<SiteAdminView>('/admin/site/logo')

/** Upload or replace the favicon as multipart/form-data (site.settings.manage). */
export const uploadAdminSiteFavicon = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return http.postForm<SiteAdminView>('/admin/site/favicon', form)
}

/** Remove the favicon; idempotent when absent (site.settings.manage). */
export const deleteAdminSiteFavicon = () => http.delete<SiteAdminView>('/admin/site/favicon')
