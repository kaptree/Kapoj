const encoder = new TextEncoder()

/** UTF-8 byte length of value.slice(0, utf16Offset). */
export function utf8Offset(value: string, utf16Offset: number): number {
  return encoder.encode(value.slice(0, utf16Offset)).length
}
