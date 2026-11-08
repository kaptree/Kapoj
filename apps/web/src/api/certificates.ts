import { http } from './client'
import type { CertificateTypeView, CertificateView, Page, PublicCertificateView } from './types'

export interface SubmitCertificateInput {
  typeId: number
  level: string
  file: File
}

/** Enabled certificate types (public). */
export const listCertificateTypes = () => http.get<CertificateTypeView[]>('/certificate-types')

/** Submit a certificate application as multipart/form-data. */
export const submitCertificate = (input: SubmitCertificateInput) => {
  const form = new FormData()
  form.append('typeId', String(input.typeId))
  form.append('level', input.level)
  form.append('file', input.file)
  return http.postForm<CertificateView>('/certificates', form)
}

/** The current user's certificate page (newest first). */
export const listMyCertificates = (page = 1, pageSize = 20) =>
  http.get<Page<CertificateView>>('/certificates/my', {
    params: { page, page_size: pageSize },
  })

/** Raw certificate image (Bearer + refresh); the caller owns the Blob's lifetime. */
export const getCertificateImage = (id: number | string) =>
  http.getBlob(`/certificates/${id}/image`)

/** Public verification; unknown/pending/rejected resolve to `{valid:false}` (200). */
export const verifyCertificate = (code: string) =>
  http.get<PublicCertificateView>('/certificates/verify', { params: { code } })
