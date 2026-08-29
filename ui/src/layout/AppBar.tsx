import React, { createElement, forwardRef } from 'react'
import {
  AppBar as RAAppBar,
  MenuItemLink,
  useTranslate,
  usePermissions,
  useResourceDefinitions,
} from 'react-admin'
import type { ResourceDefinition } from 'react-admin'
import { MdInfo, MdPerson, MdSupervisorAccount } from 'react-icons/md'
import { MenuItem, ListItemIcon, Divider } from '@mui/material'
import ViewListIcon from '@mui/icons-material/ViewList'
import { Dialogs } from '../dialogs/Dialogs'
import { AboutDialog } from '../dialogs'
import PersonalMenu from './PersonalMenu'
import ActivityPanel from './ActivityPanel'
import NowPlayingPanel from './NowPlayingPanel'
import UserMenu from './UserMenu'
import Logout from './Logout'
import config from '../config'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const menuItemSx = {
  color: 'text.secondary',
  '&.RaMenuItemLink-active': { color: 'text.primary' },
}

type AboutMenuItemProps = {
  onClick?: () => void
}

const AboutMenuItem = forwardRef<HTMLLIElement, AboutMenuItemProps>(
  ({ onClick, ...rest }, ref) => {
    const translate = useTranslate()
    const [open, setOpen] = React.useState(false)

    const handleOpen = () => setOpen(true)
    const handleClose = () => {
      onClick?.()
      setOpen(false)
    }
    const label = translate('menu.about')

    return (
      <>
        <MenuItem
          ref={ref}
          onClick={handleOpen}
          sx={[
            menuItemSx,
            (theme) => componentStyleOverride(theme, 'NDAppBar', 'root'),
          ]}
        >
          <ListItemIcon
            sx={[
              { minWidth: 5 },
              (theme) => componentStyleOverride(theme, 'NDAppBar', 'icon'),
            ]}
          >
            <MdInfo title={label} size={24} />
          </ListItemIcon>
          {label}
        </MenuItem>
        <AboutDialog onClose={handleClose} open={open} />
      </>
    )
  },
)

AboutMenuItem.displayName = 'AboutMenuItem'

const settingsResources = (resource: ResourceDefinition) =>
  resource.name !== 'user' &&
  resource.hasList &&
  resource.options &&
  resource.options.subMenu === 'settings'

const CustomUserMenu = (props) => {
  const translate = useTranslate()
  const resourceDefinitions = useResourceDefinitions()
  const resources = Object.values(resourceDefinitions ?? {})
  const { permissions } = usePermissions()

  const resourceDefinition = (resourceName) =>
    resources.find((resource) => resource?.name === resourceName)

  const renderSettingsMenuItemLink = (
    resource: ResourceDefinition,
    id?: string | null,
  ) => {
    const label = translate(`resources.${resource.name}.name`, {
      smart_count: id ? 1 : 2,
    })
    const link = id ? `/${resource.name}/${id}` : `/${resource.name}`
    return (
      <MenuItemLink
        sx={[
          menuItemSx,
          (theme) => componentStyleOverride(theme, 'NDAppBar', 'root'),
        ]}
        key={resource.name}
        to={link}
        primaryText={label}
        leftIcon={
          (resource.icon && createElement(resource.icon, { size: 24 })) || (
            <ViewListIcon />
          )
        }
        sidebarIsOpen={true}
      />
    )
  }

  const renderUserMenuItemLink = () => {
    const resource = resourceDefinition('user')
    if (!resource) return null
    if (permissions !== 'admin' && !config.enableUserEditing) return null

    const userResource = {
      ...resource,
      icon: permissions === 'admin' ? MdSupervisorAccount : MdPerson,
    }
    return renderSettingsMenuItemLink(
      userResource,
      permissions !== 'admin' ? localStorage.getItem('userId') : null,
    )
  }

  return (
    <>
      {config.devActivityPanel &&
        permissions === 'admin' &&
        config.enableNowPlaying && <NowPlayingPanel />}
      {config.devActivityPanel && permissions === 'admin' && <ActivityPanel />}
      <UserMenu {...props}>
        <PersonalMenu sidebarIsOpen={true} dense={false} />
        <Divider />
        {renderUserMenuItemLink()}
        {resources
          .filter(settingsResources)
          .map((resource) => renderSettingsMenuItemLink(resource, null))}
        <Divider />
        <AboutMenuItem />
        {(!config.auth || !!config.extAuthLogoutURL) && <Logout />}
      </UserMenu>
      <Dialogs />
    </>
  )
}

const AppBarContainer = ({ children }) => <>{children}</>

const AppBar = (props) => (
  <RAAppBar
    {...props}
    container={AppBarContainer}
    userMenu={<CustomUserMenu />}
  />
)

export default AppBar
