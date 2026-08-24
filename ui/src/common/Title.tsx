import React from 'react'
import { useMediaQuery } from '@mui/material'
import { useTranslate } from 'react-admin'

type TitleProps = {
  subTitle: string
  args?: Record<string, unknown>
}

export const Title = ({ subTitle, args }: TitleProps) => {
  const translate = useTranslate()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const text = translate(subTitle, { ...args, _: subTitle })

  if (isDesktop) {
    return <span>Navidrome {text ? ` - ${text}` : ''}</span>
  }
  return <span>{text ? text : 'Navidrome'}</span>
}
