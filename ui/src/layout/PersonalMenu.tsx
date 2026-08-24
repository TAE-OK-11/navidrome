// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { forwardRef } from 'react'
import { MenuItemLink, useTranslate } from 'react-admin'
import { MdTune } from 'react-icons/md'

const PersonalMenu = forwardRef(({ onClick, sidebarIsOpen, dense }, ref) => {
  const translate = useTranslate()
  return (
    <MenuItemLink
      ref={ref}
      to="/personal"
      primaryText={translate('menu.personal.name')}
      leftIcon={<MdTune size={24} />}
      onClick={onClick}
      sx={{ color: 'text.secondary' }}
      sidebarIsOpen={sidebarIsOpen}
      dense={dense}
    />
  )
})

PersonalMenu.displayName = 'PersonalMenu'

export default PersonalMenu
