import React, { useState } from 'react'
import { Box, Typography, Collapse } from '@mui/material'
import Card from '@mui/material/Card'
import CardMedia from '@mui/material/CardMedia'
import config from '../config'
import {
  LoveButton,
  RatingField,
  ImageUploadOverlay,
  useImageLoadingState,
} from '../common'
import ImageLightbox from '../common/ImageLightbox'
import subsonic from '../subsonic'
import { SafeHTML } from '../common/SafeHTML'
import { componentStyleOverride } from '../themes/componentStyleOverride'
import type { ArtistRecord } from '../types/records'

const mobileOverride = (slot) => (theme) =>
  componentStyleOverride(theme, 'NDMobileArtistDetails', slot)

type ArtistInfo = {
  biography?: string
}

type MobileArtistDetailsProps = {
  artistInfo?: ArtistInfo
  biography?: string
  record: ArtistRecord
}

const MobileArtistDetails = ({
  artistInfo,
  biography,
  record,
}: MobileArtistDetailsProps) => {
  const img = subsonic.getCoverArtUrl(record, 800)
  const [expanded, setExpanded] = useState(false)
  const title = record.name
  const {
    imageLoading,
    imageError,
    isLightboxOpen,
    handleImageLoad,
    handleImageError,
    handleOpenLightbox,
    handleCloseLightbox,
  } = useImageLoadingState(record.id)

  return (
    <>
      <Box
        sx={[
          { display: 'flex', background: `url(${img})` },
          mobileOverride('root'),
        ]}
      >
        <Box
          sx={[
            {
              display: 'flex',
              height: '15rem',
              width: '100vw',
              p: 'unset',
              backdropFilter: 'blur(1px)',
              backgroundPosition: '50% 30%',
              background:
                'linear-gradient(to bottom, rgba(52 52 52 / 72%), rgba(21 21 21))',
            },
            mobileOverride('bgContainer'),
          ]}
        >
          <Card
            sx={[
              {
                ml: '1em',
                maxHeight: '7rem',
                bgcolor: 'inherit',
                mt: '4rem',
                width: '7rem',
                minWidth: '7rem',
                display: 'flex',
                borderRadius: '5em',
                position: 'relative',
              },
              mobileOverride('artistImage'),
            ]}
          >
            {artistInfo && (
              <CardMedia
                key={record.id}
                component="img"
                src={subsonic.getCoverArtUrl(record, config.uiCoverArtSize)}
                sx={[
                  {
                    width: 151,
                    boxShadow: '0px 0px 6px 0px #565656',
                    borderRadius: '5px',
                    bgcolor: 'transparent',
                    transition: 'opacity 0.3s ease-in-out',
                    objectFit: 'cover',
                    opacity: imageLoading ? 0.5 : 1,
                    cursor: imageError ? 'default' : 'pointer',
                  },
                  mobileOverride('cover'),
                  imageLoading && mobileOverride('coverLoading'),
                ]}
                onClick={handleOpenLightbox}
                onLoad={handleImageLoad}
                onError={handleImageError}
                title={title}
              />
            )}
            <ImageUploadOverlay
              entityType="artist"
              entityId={record.id}
              hasUploadedImage={!!record.uploadedImage}
            />
          </Card>
          <Box
            sx={[
              {
                display: 'flex',
                alignItems: 'flex-start',
                flexDirection: 'column',
                justifyContent: 'center',
                ml: '0.5rem',
              },
              mobileOverride('details'),
            ]}
          >
            <Typography
              component="h5"
              variant="h5"
              sx={[{ wordBreak: 'break-word' }, mobileOverride('artistName')]}
            >
              {title}
              <LoveButton
                sx={[{ top: -0.2, left: 0.5 }, mobileOverride('loveButton')]}
                record={record}
                resource={'artist'}
                size={'small'}
                aria-label="love"
                color="primary"
              />
            </Typography>
            {config.enableStarRating && (
              <RatingField
                record={record}
                resource={'artist'}
                size={'small'}
                sx={[{ mt: '5px' }, mobileOverride('rating')]}
              />
            )}
          </Box>
        </Box>
      </Box>
      <Box
        sx={[
          {
            display: 'flex',
            mx: '3%',
            mt: '-2em',
            zIndex: 1,
            '& p': {
              whiteSpace: expanded ? 'unset' : 'nowrap',
              overflow: 'hidden',
              width: '95vw',
              textOverflow: 'ellipsis',
            },
          },
          mobileOverride('biography'),
        ]}
      >
        <Collapse collapsedSize={'1.5em'} in={expanded} timeout={'auto'}>
          <Typography variant={'body1'} onClick={() => setExpanded(!expanded)}>
            <span>
              <SafeHTML>{biography}</SafeHTML>
            </span>
          </Typography>
        </Collapse>
      </Box>
      <ImageLightbox
        open={isLightboxOpen && !imageError}
        imageUrl={img}
        title={record.name}
        onClose={handleCloseLightbox}
      />
    </>
  )
}

export default MobileArtistDetails
