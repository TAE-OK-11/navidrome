// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import * as React from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import StarIcon from '@mui/icons-material/Star'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import DynamicMenuIcon from './DynamicMenuIcon'

describe('<DynamicMenuIcon />', () => {
  afterEach(cleanup)

  it('renders icon if no activeIcon is specified', () => {
    const route = '/test'

    render(
      <MemoryRouter initialEntries={[route]}>
        <DynamicMenuIcon icon={StarIcon} path={'test'} />
      </MemoryRouter>,
    )
    expect(screen.getByTestId('icon')).not.toBeNull()
  })

  it('renders icon if path does not match the URL', () => {
    const route = '/path'

    render(
      <MemoryRouter initialEntries={[route]}>
        <DynamicMenuIcon
          icon={StarIcon}
          activeIcon={StarBorderIcon}
          path={'otherpath'}
        />
      </MemoryRouter>,
    )
    expect(screen.getByTestId('icon')).not.toBeNull()
  })

  it('renders activeIcon if path matches the URL', () => {
    const route = '/path'

    render(
      <MemoryRouter initialEntries={[route]}>
        <DynamicMenuIcon
          icon={StarIcon}
          activeIcon={StarBorderIcon}
          path={'path'}
        />
      </MemoryRouter>,
    )
    expect(screen.getByTestId('activeIcon')).not.toBeNull()
  })
})
