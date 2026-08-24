// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import { Box, useMediaQuery } from '@mui/material'
import { Link } from 'react-router-dom'
import { QualityInfo } from '../common'
import { decisionService } from '../transcode'
import { useDrag } from 'react-dnd'
import { DraggableTypes } from '../consts'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const audioOverride = (slot) => (theme) =>
  componentStyleOverride(theme, 'NDAudioPlayer', slot)

const AudioTitle = React.memo(({ audioInfo, gainInfo, isMobile }) => {
  const isDesktop = useMediaQuery('(min-width:810px)')

  const song = audioInfo.song
  const [, dragSongRef] = useDrag(
    () => ({
      type: DraggableTypes.SONG,
      item: { ids: [song?.id] },
      options: { dropEffect: 'copy' },
    }),
    [song],
  )

  if (!song) {
    return ''
  }

  const qi = {
    suffix: song.suffix,
    bitRate: song.bitRate,
    rgAlbumGain: song.rgAlbumGain,
    rgAlbumPeak: song.rgAlbumPeak,
    rgTrackGain: song.rgTrackGain,
    rgTrackPeak: song.rgTrackPeak,
  }

  const decision = decisionService.getCachedDecision(audioInfo.trackId)
  const transcodeProps = decision
    ? {
        transcodeStream: decision.transcodeStream || null,
        isDirectPlay: decision.canDirectPlay,
      }
    : {}

  const subtitle = song.tags?.['subtitle']
  const title = song.title + (subtitle ? ` (${subtitle})` : '')

  const linkTo = audioInfo.isRadio
    ? `/radio/${audioInfo.trackId}/show`
    : song.playlistId
      ? `/playlist/${song.playlistId}/show`
      : `/album/${song.albumId}/show`

  return (
    <Box
      component={Link}
      to={linkTo}
      ref={dragSongRef}
      sx={[
        { textDecoration: 'none', color: 'primary.dark' },
        audioOverride('audioTitle'),
      ]}
    >
      <span>
        <Box
          component="span"
          className="songTitle"
          sx={[
            {
              fontWeight: 'bold',
              '&:hover + .quality-info': { opacity: 1 },
            },
            audioOverride('songTitle'),
          ]}
        >
          {title}
        </Box>
        {isDesktop && (
          <QualityInfo
            record={qi}
            className="quality-info"
            sx={[
              {
                mt: '-4px',
                opacity: 0,
                transition: 'all 500ms ease-out',
              },
              audioOverride('qualityInfo'),
            ]}
            {...gainInfo}
            {...transcodeProps}
          />
        )}
      </span>
      {isMobile ? (
        <>
          <Box
            component="span"
            sx={[{ display: 'block', mt: '2px' }, audioOverride('songInfo')]}
          >
            <span className={'songArtist'}>{song.artist}</span>
          </Box>
          <Box
            component="span"
            sx={[
              {
                display: 'block',
                mt: '2px',
                fontStyle: 'italic',
                fontSize: 'smaller',
              },
              audioOverride('songInfo'),
              audioOverride('songAlbum'),
            ]}
          >
            <span className={'songAlbum'}>{song.album}</span>
            {song.year ? ` - ${song.year}` : ''}
          </Box>
        </>
      ) : (
        <Box
          component="span"
          sx={[{ display: 'block', mt: '2px' }, audioOverride('songInfo')]}
        >
          <span className={'songArtist'}>{song.artist}</span> -{' '}
          <span className={'songAlbum'}>{song.album}</span>
          {song.year ? ` - ${song.year}` : ''}
        </Box>
      )}
    </Box>
  )
})

AudioTitle.displayName = 'AudioTitle'

export default AudioTitle
