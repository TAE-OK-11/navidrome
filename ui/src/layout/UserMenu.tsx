// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import * as React from 'react'
import {
  Children,
  cloneElement,
  isValidElement,
  useEffect,
  useState,
} from 'react'
import PropTypes from 'prop-types'
import { useTranslate, useGetIdentity } from 'react-admin'
import {
  Tooltip,
  IconButton,
  Popover,
  MenuList,
  Avatar,
  Card,
  CardContent,
  Divider,
  Typography,
  Box,
} from '@mui/material'
import AccountCircle from '@mui/icons-material/AccountCircle'
import config from '../config'
import authProvider from '../authProvider'
import { startEventStream } from '../eventStream'
import { useDispatch } from 'react-redux'

const UserMenu = ({
  children,
  label = 'menu.settings',
  icon = <AccountCircle />,
  ...props
}) => {
  const [anchorEl, setAnchorEl] = useState(null)
  const translate = useTranslate()
  const { data: identity, isPending } = useGetIdentity()
  const dispatch = useDispatch()

  useEffect(() => {
    if (config.devActivityPanel) {
      authProvider
        .checkAuth()
        .then(() => startEventStream(dispatch))
        .catch(() => {})
    }
  }, [dispatch])

  if (!children) return null
  const open = Boolean(anchorEl)
  const loaded = !isPending && Boolean(identity)

  const handleMenu = (event) => setAnchorEl(event.currentTarget)
  const handleClose = () => setAnchorEl(null)

  return (
    <Box>
      <Tooltip title={label && translate(label, { _: label })}>
        <IconButton
          sx={{ color: 'inherit' }}
          aria-label={label && translate(label, { _: label })}
          aria-controls={open ? 'menu-appbar' : undefined}
          aria-haspopup="true"
          onClick={handleMenu}
          size="large"
        >
          {loaded && identity?.avatar ? (
            <Avatar
              sx={{ width: 4, height: 4 }}
              src={identity.avatar}
              alt={identity.fullName || ''}
            />
          ) : (
            icon
          )}
        </IconButton>
      </Tooltip>
      <Popover
        id="menu-appbar"
        anchorEl={anchorEl}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        open={open}
        onClose={handleClose}
      >
        <MenuList>
          {loaded && (
            <Card
              elevation={0}
              sx={{ maxWidth: '11em', mt: '-0.7em', mb: '-1em' }}
            >
              <CardContent
                sx={{
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                }}
              >
                <Typography variant="button">{identity.fullName}</Typography>
              </CardContent>
            </Card>
          )}
          <Divider />
          {Children.map(children, (menuItem) =>
            isValidElement(menuItem)
              ? cloneElement(menuItem, {
                  onClick: handleClose,
                })
              : null,
          )}
        </MenuList>
      </Popover>
    </Box>
  )
}

UserMenu.propTypes = {
  children: PropTypes.node,
  label: PropTypes.string,
  icon: PropTypes.node,
}

export default UserMenu
