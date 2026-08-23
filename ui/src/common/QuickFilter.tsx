import type { ReactNode } from 'react'
import { Chip } from '@mui/material'
import { useTranslate } from 'react-admin'
import { humanize, underscore } from 'inflection'

type QuickFilterProps = {
  source: string
  resource: string
  label?: ReactNode
  defaultValue?: unknown
}

export const QuickFilter = ({ source, resource, label }: QuickFilterProps) => {
  const translate = useTranslate()
  let lbl: ReactNode = label || source
  if (typeof lbl === 'string') {
    if (label) {
      lbl = translate(lbl, {
        _: humanize(underscore(lbl)),
      })
    } else {
      lbl = translate(`resources.${resource}.fields.${source}`, {
        _: humanize(underscore(source)),
      })
    }
  }
  return <Chip sx={{ mb: 1 }} label={lbl} />
}
