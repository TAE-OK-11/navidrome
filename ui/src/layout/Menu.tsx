import React, { useState } from 'react'
import { useSelector } from 'react-redux'
import { Divider, MenuList } from '@mui/material'
import {
  useTranslate,
  MenuItemLink,
  useResourceDefinitions,
  useSidebarState,
} from 'react-admin'
import ViewListIcon from '@mui/icons-material/ViewList'
import AlbumIcon from '@mui/icons-material/Album'
import SubMenu from './SubMenu'
import { humanize, pluralize } from 'inflection'
import albumLists from '../album/albumLists'
import PlaylistsSubMenu from './PlaylistsSubMenu'
import LibrarySelector from '../common/LibrarySelector'
import config from '../config'

const translatedResourceName = (resource, translate) =>
  translate(`resources.${resource.name}.name`, {
    smart_count: 2,
    _:
      resource.options && resource.options.label
        ? translate(resource.options.label, {
            smart_count: 2,
            _: resource.options.label,
          })
        : humanize(pluralize(resource.name)),
  })

const Menu = ({ dense = false }) => {
  const [open] = useSidebarState()
  const translate = useTranslate()
  const queue = useSelector((state) => state.player?.queue || [])
  const resourceDefinitions = useResourceDefinitions()
  const resources = Object.values(resourceDefinitions ?? {})

  const [state, setState] = useState({
    menuAlbumList: true,
    menuPlaylists: true,
    menuSharedPlaylists: true,
  })

  const handleToggle = (menu) => {
    setState((state) => ({ ...state, [menu]: !state[menu] }))
  }

  const renderResourceMenuItemLink = (resource) => (
    <MenuItemLink
      key={resource.name}
      to={`/${resource.name}`}
      primaryText={translatedResourceName(resource, translate)}
      leftIcon={resource.icon || <ViewListIcon />}
      sidebarIsOpen={open}
      dense={dense}
    />
  )

  const renderAlbumMenuItemLink = (type, al) => {
    const resource = resources.find((r) => r.name === 'album')
    if (!resource) {
      return null
    }

    const albumListAddress = `/album/${type}`
    const name = translate(`resources.album.lists.${type || 'default'}`, {
      _: translatedResourceName(resource, translate),
    })

    return (
      <MenuItemLink
        key={albumListAddress}
        to={albumListAddress}
        primaryText={name}
        leftIcon={al.icon || <ViewListIcon />}
        sidebarIsOpen={open}
        dense={dense}
      />
    )
  }

  const subItems = (subMenu) => (resource) =>
    resource.hasList && resource.options && resource.options.subMenu === subMenu

  return (
    <MenuList
      sx={(theme) => ({
        mt: 1,
        mb: 1,
        width: open ? 240 : 55,
        pb: queue.length > 0 ? '80px' : '20px',
        transition: theme.transitions.create('width', {
          easing: theme.transitions.easing.sharp,
          duration: theme.transitions.duration.leavingScreen,
        }),
        '& .RaMenuItemLink-active': {
          color: 'text.primary',
          fontWeight: 'bold',
        },
      })}
      component="nav"
      disablePadding
    >
      {open && <LibrarySelector />}
      <SubMenu
        handleToggle={() => handleToggle('menuAlbumList')}
        isOpen={state.menuAlbumList}
        sidebarIsOpen={open}
        name="menu.albumList"
        icon={<AlbumIcon />}
        dense={dense}
      >
        {Object.keys(albumLists).map((type) =>
          renderAlbumMenuItemLink(type, albumLists[type]),
        )}
      </SubMenu>
      {resources.filter(subItems(undefined)).map(renderResourceMenuItemLink)}
      {config.devSidebarPlaylists && open ? (
        <>
          <Divider />
          <PlaylistsSubMenu
            state={state}
            setState={setState}
            sidebarIsOpen={open}
            dense={dense}
          />
        </>
      ) : (
        resources.filter(subItems('playlist')).map(renderResourceMenuItemLink)
      )}
    </MenuList>
  )
}

export default Menu
