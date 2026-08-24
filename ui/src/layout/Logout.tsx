import React, { forwardRef, useCallback } from 'react'
import { useDispatch } from 'react-redux'
import { useLogout, useTranslate } from 'react-admin'
import { ListItemIcon, ListItemText, MenuItem } from '@mui/material'
import type { MenuItemProps } from '@mui/material'
import PowerSettingsNewIcon from '@mui/icons-material/PowerSettingsNew'
import { clearQueue } from '../actions'

type LogoutProps = Omit<MenuItemProps, 'onClick'> & {
  onClick?: React.MouseEventHandler<HTMLLIElement>
}

const Logout = forwardRef<HTMLLIElement, LogoutProps>(
  ({ onClick, ...props }, ref) => {
    const dispatch = useDispatch()
    const logout = useLogout()
    const translate = useTranslate()

    const handleClick = useCallback(
      (event) => {
        dispatch(clearQueue())
        onClick?.(event)
        logout()
      },
      [dispatch, logout, onClick],
    )

    return (
      <MenuItem ref={ref} onClick={handleClick} {...props}>
        <ListItemIcon>
          <PowerSettingsNewIcon fontSize="small" />
        </ListItemIcon>
        <ListItemText>{translate('ra.auth.logout')}</ListItemText>
      </MenuItem>
    )
  },
)

Logout.displayName = 'Logout'

export default Logout
