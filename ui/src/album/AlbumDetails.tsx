import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import {
  Card,
  CardContent,
  CardMedia,
  Collapse,
  Box,
  Typography,
  useMediaQuery,
} from '@mui/material'
import {
  ArrayField,
  ChipField,
  Link,
  SingleFieldList,
  useRecordContext,
  useTranslate,
  type Identifier,
} from 'react-admin'
import ImageLightbox from '../common/ImageLightbox'
import config from '../config'
import subsonic from '../subsonic'
import {
  ArtistLinkField,
  CollapsibleComment,
  DurationField,
  formatRange,
  LoveButton,
  RatingField,
  SizeField,
  useAlbumsPerPage,
  useImageLoadingState,
} from '../common'
import { formatFullDate, intersperse } from '../utils'
import AlbumExternalLinks from './AlbumExternalLinks'
import { SafeHTML } from '../common/SafeHTML'
import { withWidth, type Width } from '../themes/useWidth'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { Theme } from '@mui/material/styles'
import type { AlbumRecord } from '../types/records'

const albumDetailsSx = (slot: string, styles: (theme: Theme) => object) => (theme: Theme) => ({
  ...styles(theme),
  ...componentStyleOverride(theme, 'NDAlbumDetails', slot),
})

const notesSx = albumDetailsSx('notes', () => ({
  display: 'inline-block',
  mt: '1em',
  float: 'left',
  wordBreak: 'break-word',
  cursor: 'pointer',
}))

const useGetHandleGenreClick = (width?: Width) => {
  const [perPage] = useAlbumsPerPage(width)

  return (id: Identifier) => {
    return `/album?filter={"genre_id":["${id}"]}&order=ASC&sort=name&perPage=${perPage}`
  }
}

const GenreChipField = withWidth()(({ width, ...rest }) => {
  const record = useRecordContext<{ id: Identifier }>(rest)
  const genreLink = useGetHandleGenreClick(width)

  if (!record) return null

  return (
    <Link to={genreLink(record.id)} onClick={(e) => e.stopPropagation()}>
      <ChipField
        source="name"
        // Workaround to force ChipField to be clickable
        onClick={() => {}}
      />
    </Link>
  )
})

const GenreList = () => {
  return (
    <ArrayField source={'genres'}>
      <SingleFieldList linkType={false}>
        <GenreChipField />
      </SingleFieldList>
    </ArrayField>
  )
}

type DetailsProps = {
  record?: AlbumRecord
}

export const Details = (props: DetailsProps) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  const translate = useTranslate()
  const record = useRecordContext<AlbumRecord>(props)

  if (!record) return null

  const details: ReactNode[] = []
  const addDetail = (obj: ReactNode) => {
    const id = details.length
    details.push(<span key={`detail-${record.id}-${id}`}>{obj}</span>)
  }

  const yearRange = formatRange(record, 'year')
  const date = record.date ? formatFullDate(record.date) : yearRange

  const originalDate = record.originalDate
    ? formatFullDate(record.originalDate)
    : formatRange(record, 'originalYear')
  const releaseDate = record?.releaseDate && formatFullDate(record.releaseDate)

  const dateToUse = originalDate || date
  const isOriginalDate = originalDate && dateToUse !== date
  const showDate = dateToUse && dateToUse !== releaseDate

  const getDateLabel = () => {
    if (isXsmall) return '♫'
    if (isOriginalDate) return translate('resources.album.fields.originalDate')
    return null
  }

  const getReleaseDateLabel = () => {
    if (!isXsmall) return translate('resources.album.fields.releaseDate')
    if (showDate) return '○'
    return null
  }

  if (showDate) {
    addDetail(<>{[getDateLabel(), dateToUse].filter(Boolean).join('  ')}</>)
  }

  if (releaseDate) {
    addDetail(
      <>{[getReleaseDateLabel(), releaseDate].filter(Boolean).join('  ')}</>,
    )
  }
  addDetail(
    <>
      {record.songCount +
        ' ' +
        translate('resources.song.name', {
          smart_count: record.songCount,
        })}
    </>,
  )
  !isXsmall && addDetail(<DurationField source={'duration'} />)
  !isXsmall && addDetail(<SizeField source="size" />)

  return <>{intersperse(details, ' · ')}</>
}

type AlbumInfoData = {
  notes?: string
}

type AlbumDetailsProps = {
  record?: AlbumRecord
}

const AlbumDetails = (props: AlbumDetailsProps) => {
  const record = useRecordContext<AlbumRecord>(props)
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('lg'))
  const [expanded, setExpanded] = useState(false)
  const [albumInfo, setAlbumInfo] = useState<AlbumInfoData>()
  const {
    imageLoading,
    imageError,
    isLightboxOpen,
    handleImageLoad,
    handleImageError,
    handleOpenLightbox,
    handleCloseLightbox,
  } = useImageLoadingState(record?.id)

  useEffect(() => {
    if (!record?.id) return
    subsonic
      .getAlbumInfo(String(record.id))
      .then((resp) => resp.json['subsonic-response'])
      .then((data) => {
        if (data.status === 'ok') {
          setAlbumInfo(data.albumInfo)
        }
      })
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.error('error on album page', e)
      })
  }, [record])

  if (!record) return null

  let notes = albumInfo?.notes || record.notes

  if (notes) {
    notes += '..'
  }

  const imageUrl = subsonic.getCoverArtUrl(record, config.uiCoverArtSize)
  const fullImageUrl = subsonic.getCoverArtUrl(record)

  return (
    <Card
      sx={albumDetailsSx('root', (theme) => ({
        width: '100%',
        overflow: 'hidden',
        borderRadius: 5,
        background: `linear-gradient(135deg, ${theme.palette.background.paper}, ${theme.palette.action.hover})`,
        [theme.breakpoints.down('sm')]: { p: 1.5, minWidth: 0 },
        [theme.breakpoints.up('sm')]: { p: 2, minWidth: 0 },
      }))}
    >
      <Box
        sx={albumDetailsSx('cardContents', (theme) => ({
          display: 'grid',
          gridTemplateColumns: 'minmax(7.5rem, 15rem) minmax(0, 1fr)',
          gap: 2,
          alignItems: 'start',
          [theme.breakpoints.down('sm')]: {
            gridTemplateColumns: '7.5rem minmax(0, 1fr)',
            gap: 1.5,
          },
        }))}
      >
        <Box
          sx={albumDetailsSx('coverParent', (theme) => ({
            [theme.breakpoints.down('sm')]: {
              height: '7.5rem',
              width: '7.5rem',
              minWidth: '7.5rem',
            },
            [theme.breakpoints.up('sm')]: {
              height: '10em',
              width: '10em',
              minWidth: '10em',
            },
            [theme.breakpoints.up('lg')]: {
              height: '15em',
              width: '15em',
              minWidth: '15em',
            },
            backgroundColor: 'transparent',
            overflow: 'hidden',
            borderRadius: 4,
            boxShadow: '0 10px 24px rgba(0, 0, 0, 0.22)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }))}
        >
          <CardMedia
            key={record.id}
            component={'img'}
            src={imageUrl}
            width="400"
            height="400"
            sx={albumDetailsSx('cover', () => ({
              objectFit: 'cover',
              cursor: imageError ? 'default' : 'pointer',
              display: 'block',
              width: '100%',
              height: '100%',
              backgroundColor: 'transparent',
              opacity: imageLoading ? 0.5 : 1,
              transition: 'opacity 0.3s ease-in-out, transform 250ms ease',
              '&:hover': { transform: 'scale(1.025)' },
            }))}
            onClick={handleOpenLightbox}
            onLoad={handleImageLoad}
            onError={handleImageError}
            title={record.name}
          />
        </Box>
        <Box
          sx={albumDetailsSx('details', () => ({
            display: 'flex',
            flexDirection: 'column',
          }))}
        >
          <CardContent
            sx={albumDetailsSx('content', () => ({ flex: '2 0 auto' }))}
          >
            <Typography
              variant={isDesktop ? 'h5' : 'h6'}
              sx={albumDetailsSx('recordName', () => ({
                fontWeight: 750,
                lineHeight: 1.18,
                letterSpacing: '-0.02em',
                overflowWrap: 'anywhere',
              }))}
            >
              {record.name}
              <LoveButton
                sx={albumDetailsSx('loveButton', () => ({
                  ml: 0.5,
                  verticalAlign: 'middle',
                }))}
                record={record}
                resource={'album'}
                size={isDesktop ? 'default' : 'small'}
                aria-label="love"
                color="primary"
              />
            </Typography>
            <Typography
              component={'h6'}
              sx={albumDetailsSx('recordArtist', (theme) => ({
                mt: 0.5,
                color: theme.palette.text.secondary,
              }))}
            >
              {record?.tags?.['albumversion']}
            </Typography>
            <Typography
              component={'h6'}
              sx={albumDetailsSx('recordArtist', (theme) => ({
                mt: 0.5,
                color: theme.palette.text.secondary,
              }))}
            >
              <ArtistLinkField record={record} />
            </Typography>
            <Typography
              component={'div'}
              sx={albumDetailsSx('recordMeta', (theme) => ({
                mt: 1,
                color: theme.palette.text.secondary,
                lineHeight: 1.6,
              }))}
            >
              <Details record={record} />
            </Typography>
            {config.enableStarRating && (
              <div>
                <RatingField
                  record={record}
                  resource={'album'}
                  size={isDesktop ? 'medium' : 'small'}
                />
              </div>
            )}
            {isDesktop ? (
              <GenreList />
            ) : (
              <Typography component={'p'}>{record.genre}</Typography>
            )}
            {!isXsmall && (
              <Typography
                component={'div'}
                sx={albumDetailsSx('recordMeta', (theme) => ({
                  mt: 1,
                  color: theme.palette.text.secondary,
                  lineHeight: 1.6,
                }))}
              >
                {config.enableExternalServices && (
                  <Box
                    sx={albumDetailsSx('externalLinks', () => ({ mt: 1.5 }))}
                  >
                    <AlbumExternalLinks />
                  </Box>
                )}
              </Typography>
            )}
            {isDesktop && notes && (
              <Collapse
                collapsedSize={'2.75em'}
                in={expanded}
                timeout={'auto'}
                sx={notesSx}
              >
                <Typography
                  variant={'body1'}
                  onClick={() => setExpanded(!expanded)}
                >
                  <span>
                    <SafeHTML>{notes}</SafeHTML>
                  </span>
                </Typography>
              </Collapse>
            )}
            {isDesktop && record['comment'] && (
              <CollapsibleComment
                record={{ id: String(record.id), comment: record.comment }}
              />
            )}
          </CardContent>
        </Box>
      </Box>
      {!isDesktop && record['comment'] && (
        <CollapsibleComment
          record={{ id: String(record.id), comment: record.comment }}
        />
      )}
      {!isDesktop && notes && (
        <Box sx={notesSx}>
          <Collapse collapsedSize={'1.5em'} in={expanded} timeout={'auto'}>
            <Typography
              variant={'body1'}
              onClick={() => setExpanded(!expanded)}
            >
              <span>
                <SafeHTML>{notes}</SafeHTML>
              </span>
            </Typography>
          </Collapse>
        </Box>
      )}
      <ImageLightbox
        open={isLightboxOpen && !imageError}
        imageUrl={fullImageUrl}
        title={record.name}
        onClose={handleCloseLightbox}
      />
    </Card>
  )
}

export default AlbumDetails
