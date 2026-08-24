// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { Fragment } from 'react'
import ExpandMore from '@mui/icons-material/ExpandMore'
import ArrowRightOutlined from '@mui/icons-material/ArrowRightOutlined'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import Typography from '@mui/material/Typography'
import Collapse from '@mui/material/Collapse'
import Tooltip from '@mui/material/Tooltip'
import { useSidebarState, useTranslate } from 'react-admin'
import { Box, IconButton, useMediaQuery } from '@mui/material'

const SubMenu = ({
  handleToggle,
  sidebarIsOpen,
  isOpen,
  name,
  icon,
  children,
  dense,
  onAction,
  actionIcon = <ArrowRightOutlined fontSize={'small'} />,
  onSecondaryAction,
  secondaryActionIcon,
  secondaryActionTitle,
  secondaryActionActive,
}) => {
  const translate = useTranslate()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const isSmall = useMediaQuery((theme) => theme.breakpoints.down('md'))
  const [, setSidebarOpen] = useSidebarState()

  const handleOnClick = (e) => {
    e.stopPropagation()
    onAction(e)
    if (isSmall) {
      setSidebarOpen(false)
    }
  }

  const handleSecondaryClick = (e) => {
    e.stopPropagation()
    onSecondaryAction(e)
  }

  const header = (
    <Box
      sx={{
        display: 'flex',
        '&:hover .submenu-action': { opacity: 1 },
      }}
    >
      <ListItemButton
        dense={dense}
        sx={{ width: '100%' }}
        onClick={handleToggle}
      >
        <ListItemIcon sx={{ minWidth: 5 }}>
          {isOpen ? <ExpandMore /> : icon}
        </ListItemIcon>
        <Typography
          variant="inherit"
          color="textSecondary"
          sx={{ flexGrow: 1 }}
        >
          {translate(name)}
        </Typography>
        {onSecondaryAction && sidebarIsOpen && (
          <IconButton
            size={'small'}
            title={secondaryActionTitle}
            aria-label={secondaryActionTitle}
            className={
              isDesktop && !secondaryActionActive ? 'submenu-action' : undefined
            }
            sx={
              isDesktop && !secondaryActionActive ? { opacity: 0 } : undefined
            }
            onClick={handleSecondaryClick}
          >
            {secondaryActionIcon}
          </IconButton>
        )}
        {onAction && sidebarIsOpen && (
          <IconButton
            size={'small'}
            className={isDesktop ? 'submenu-action' : undefined}
            sx={isDesktop ? { opacity: 0 } : undefined}
            onClick={handleOnClick}
          >
            {actionIcon}
          </IconButton>
        )}
      </ListItemButton>
    </Box>
  )

  return (
    <Fragment>
      {sidebarIsOpen || isOpen ? (
        header
      ) : (
        <Tooltip title={translate(name)} placement="right">
          {header}
        </Tooltip>
      )}
      <Collapse in={isOpen} timeout="auto" unmountOnExit>
        <List
          dense={dense}
          component="div"
          disablePadding
          sx={{
            '& a': {
              transition: 'padding-left 195ms cubic-bezier(0.4, 0, 0.6, 1) 0ms',
              pl: sidebarIsOpen ? 4 : 2,
            },
          }}
        >
          {children}
        </List>
      </Collapse>
    </Fragment>
  )
}

export default SubMenu
