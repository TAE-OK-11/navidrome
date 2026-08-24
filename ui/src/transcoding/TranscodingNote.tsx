import React from 'react'
import { Card, CardContent, Typography, Box } from '@mui/material'
import { useTranslate } from 'react-admin'

type InterpolateProps = {
  message: string
  field: string
  children: React.ReactNode
}

export const Interpolate = ({ message, field, children }: InterpolateProps) => {
  const split = message.split(`%{${field}}`)
  return (
    <span>
      {split[0]}
      {children}
      {split[1]}
    </span>
  )
}
export const TranscodingNote = ({ message }: { message: string }) => {
  const translate = useTranslate()
  return (
    <Card>
      <CardContent>
        <Typography>
          <Box
            component={'span'}
            sx={{
              fontWeight: 'fontWeightBold',
            }}
          >
            {translate('message.note')}:
          </Box>{' '}
          <Interpolate message={translate(message)} field={'config'}>
            <Box
              component={'span'}
              sx={{
                fontFamily: 'Monospace',
              }}
            >
              ND_ENABLETRANSCODINGCONFIG=true
            </Box>
          </Interpolate>
        </Typography>
      </CardContent>
    </Card>
  )
}
