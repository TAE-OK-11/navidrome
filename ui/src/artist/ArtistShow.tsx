import { useState, useEffect, type ReactNode } from 'react'
import { Box, useMediaQuery } from '@mui/material'
import { styled } from '@mui/material/styles'
import {
  useShowController,
  ShowContextProvider,
  useRecordContext,
  useShowContext,
  ReferenceManyField,
  Pagination,
  Title as RaTitle,
} from 'react-admin'
import subsonic from '../subsonic'
import AlbumGridView from '../album/AlbumGridView'
import MobileArtistDetails from './MobileArtistDetails'
import DesktopArtistDetails from './DesktopArtistDetails'
import {
  useAlbumsPerPage,
  useResourceRefresh,
  useScrollRestoration,
  Title,
} from '../common/index'
import ArtistActions from './ArtistActions'
import { withWidth, type Width } from '../themes/useWidth'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { ArtistRecord } from '../types/records'

const ShowArtistActions = styled(ArtistActions)(({ theme }) => ({
  width: '100%',
  justifyContent: 'flex-start',
  display: 'flex',
  padding: '0.25em 1em',
  flexWrap: 'wrap',
  overflowX: 'auto',
  [theme.breakpoints.down('sm')]: {
    paddingLeft: '0.5em',
    paddingRight: '0.5em',
    gap: '0.5em',
    justifyContent: 'space-around',
  },
  ...componentStyleOverride(theme, 'NDArtistShow', 'actions'),
}))

type ArtistInfo = {
  biography?: string
}

const ArtistDetails = () => {
  const record = useRecordContext<ArtistRecord>()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('sm'), {
    noSsr: true,
  })
  const [artistInfo, setArtistInfo] = useState<ArtistInfo>()

  useEffect(() => {
    if (!record?.id) return
    subsonic
      .getArtistInfo(String(record.id))
      .then((resp) => resp.json['subsonic-response'])
      .then((data) => {
        if (data.status === 'ok') {
          setArtistInfo(data.artistInfo)
        }
      })
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.error('error on artist page', e)
      })
  }, [record?.id])

  if (!record) return null

  const biography = artistInfo?.biography || record.biography

  const Component = isDesktop ? DesktopArtistDetails : MobileArtistDetails
  return (
    <Component artistInfo={artistInfo} record={record} biography={biography} />
  )
}

const ArtistShowLayout = (props: { width?: Width }) => {
  const showContext = useShowContext()
  const record = useRecordContext<ArtistRecord>()
  const { width } = props
  const [, perPageOptions] = useAlbumsPerPage(width)
  useResourceRefresh('artist', 'album')
  useScrollRestoration(!!record?.id)

  const maxPerPage = 90
  let perPage = -1
  let pagination: ReactNode = null

  const count = record?.stats?.['maincredit']?.albumCount || 0

  if (count > maxPerPage) {
    perPage = Math.trunc(maxPerPage / perPageOptions[0]) * perPageOptions[0]
    const rowsPerPageOptions = [1, 2, 3].map((option) =>
      Math.trunc(option * (perPage / 3)),
    )
    pagination = <Pagination rowsPerPageOptions={rowsPerPageOptions} />
  }

  return (
    <>
      {record && <RaTitle title={<Title subTitle={record.name ?? ''} />} />}
      {record && <ArtistDetails />}
      {record && (
        <Box
          sx={[
            { p: { xs: '.5rem', sm: 0 }, pl: { sm: '.75rem' } },
            (theme) =>
              componentStyleOverride(theme, 'NDArtistShow', 'actionsContainer'),
          ]}
        >
          <ShowArtistActions record={record} />
        </Box>
      )}
      {record && (
        <ReferenceManyField
          reference="album"
          target="artist_id"
          sort={{ field: 'max_year', order: 'ASC' }}
          filter={{ artist_id: record?.id }}
          perPage={perPage}
          pagination={pagination}
        >
          <AlbumGridView {...props} />
        </ReferenceManyField>
      )}
    </>
  )
}

const ArtistShow = withWidth()((props: Record<string, unknown>) => {
  const controllerProps = useShowController(props)
  return (
    <ShowContextProvider value={controllerProps}>
      <ArtistShowLayout width={props.width as Width | undefined} />
    </ShowContextProvider>
  )
})

export default ArtistShow
