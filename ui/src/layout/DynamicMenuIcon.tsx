import { useLocation } from 'react-router-dom'
import { createElement } from 'react'

const DynamicMenuIcon = ({ icon, activeIcon, path }) => {
  const location = useLocation()

  if (!activeIcon) {
    return createElement(icon, { 'data-testid': 'icon' })
  }

  return location.pathname.startsWith('/' + path)
    ? createElement(activeIcon, { 'data-testid': 'activeIcon' })
    : createElement(icon, { 'data-testid': 'icon' })
}

export default DynamicMenuIcon
