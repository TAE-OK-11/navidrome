import { useLocation } from 'react-router-dom'
import { createElement, type ElementType } from 'react'

const DynamicMenuIcon = ({
  icon,
  activeIcon,
  path,
}: {
  icon: ElementType
  activeIcon?: ElementType
  path: string
}) => {
  const location = useLocation()

  if (!activeIcon) {
    return createElement(icon, { 'data-testid': 'icon' })
  }

  return location.pathname.startsWith('/' + path)
    ? createElement(activeIcon, { 'data-testid': 'activeIcon' })
    : createElement(icon, { 'data-testid': 'icon' })
}

export default DynamicMenuIcon
