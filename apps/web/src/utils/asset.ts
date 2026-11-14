// Module-level asset base keeps presentational components free of a Pinia
// dependency. The site store synchronizes it whenever public deployment
// settings change.
let assetBase = ''

export function setAssetBase(base: string) {
  assetBase = base.trim().replace(/\/+$/, '')
}

/** Prefix a same-origin asset path with the configured assets/CDN origin. */
export function resolveAssetUrl(path: string): string {
  if (!path || path.startsWith('//') || /^[a-z][a-z\d+.-]*:/i.test(path)) return path
  if (!assetBase) return path
  return `${assetBase}/${path.replace(/^\/+/, '')}`
}

/** Test helper; not used by production code. */
export function resetAssetBase() {
  assetBase = ''
}
