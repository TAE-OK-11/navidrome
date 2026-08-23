import React from 'react'
import { Chip } from '@mui/material'
import { useTranslate } from 'react-admin'
import { humanize, underscore } from 'inflection'

export const QuickFilter = ({ source, resource, label, defaultValue }) => {
  const translate = useTranslate()
  let lbl = label || source
  if (typeof lbl === 'string' || lbl instanceof String) {
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
