declare module 'react-measure' {
  import type { ComponentType } from 'react'

  // The package does not publish declarations and injects a dynamic prop bag.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  type UntypedMeasureProps = any

  export function withContentRect(
    types?: string | string[],
  ): (
    component: ComponentType<UntypedMeasureProps>,
  ) => ComponentType<UntypedMeasureProps>
}

declare module 'jsonexport/dist' {
  interface JsonExportOptions {
    includeHeaders?: boolean
    [key: string]: unknown
  }

  type JsonExportCallback = (error: Error | null, csv: string) => void

  export default function jsonExport(
    data: ReadonlyArray<Record<string, unknown>>,
    options: JsonExportOptions,
    callback: JsonExportCallback,
  ): void
}

declare module 'lodash/merge' {
  export default function merge<T extends object, U extends object[]>(
    target: T,
    ...sources: U
  ): T & U[number]
}

declare module 'css-mediaquery' {
  export function match(
    query: string,
    values: Record<string, string | number>,
  ): boolean

  const mediaQuery: { match: typeof match }
  export default mediaQuery
}
