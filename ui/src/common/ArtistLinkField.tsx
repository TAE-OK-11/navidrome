import React from 'react'
import { Link, useRecordContext } from 'react-admin'
import { useGetHandleArtistClick } from './useGetHandleArtistClick'
import { intersperse } from '../utils/index'
import { useDispatch } from 'react-redux'
import { closeExtendedInfoDialog } from '../actions/dialogs'
import { withWidth, type WithWidthProps } from '../themes/useWidth'
import type { ArtistParticipant, ParticipantsRecord } from '../types/records'

type ArtistLink = ArtistParticipant & {
  subRole?: string
}

type ALinkProps = WithWidthProps & {
  artist: ArtistLink
  className?: string
}

// noSSR: withWidth otherwise renders null until mounted, so the artist line pops in and grows the row.
const ALink = withWidth({ noSSR: true })((props: ALinkProps) => {
  const { artist, width, ...rest } = props
  const artistLink = useGetHandleArtistClick(width)
  const dispatch = useDispatch()

  return (
    <Link
      key={artist.id}
      to={artistLink(artist.id)}
      onClick={(e) => {
        e.stopPropagation()
        dispatch(closeExtendedInfoDialog())
      }}
      {...rest}
    >
      {artist.name}
      {(artist.subroles?.length ?? 0) > 0
        ? ` (${artist.subroles!.join(', ')})`
        : ''}
    </Link>
  )
})

const parseAndReplaceArtists = (
  displayAlbumArtist: string,
  albumArtists: ArtistLink[] | undefined,
  className?: string,
): React.ReactNode[] => {
  const result: React.ReactNode[] = []
  let lastIndex = 0

  albumArtists?.forEach((artist) => {
    const index = displayAlbumArtist.indexOf(artist.name, lastIndex)
    if (index !== -1) {
      // Add text before the artist name
      if (index > lastIndex) {
        result.push(displayAlbumArtist.slice(lastIndex, index))
      }
      // Add the artist link
      result.push(
        <ALink artist={artist} className={className} key={artist.id} />,
      )
      lastIndex = index + artist.name.length
    }
  })

  if (lastIndex === 0) {
    return []
  }

  // Add any remaining text after the last artist name
  if (lastIndex < displayAlbumArtist.length) {
    result.push(displayAlbumArtist.slice(lastIndex))
  }

  return result
}

type ArtistLinkFieldProps = {
  record?: ParticipantsRecord
  className?: string
  limit?: number
  source?: string
  sortable?: boolean
  sortByOrder?: string
  label?: React.ReactNode
}

export const ArtistLinkField = ({
  record: recordOverride,
  className,
  limit = 3,
  source = 'albumArtist',
}: ArtistLinkFieldProps) => {
  const record = useRecordContext<ParticipantsRecord>({ record: recordOverride })
  if (!record) {
    return null
  }

  const role = source.toLowerCase()

  // Get artists array with fallback
  let artists = (record.participants?.[role] as ArtistLink[] | undefined) || []
  const remixers =
    role === 'artist' && record.participants?.remixer
      ? (record.participants.remixer as ArtistLink[]).slice(0, 2)
      : []

  // Use parseAndReplaceArtists for artist and albumartist roles
  const sourceValue = record[source] as string | undefined
  if ((role === 'artist' || role === 'albumartist') && sourceValue) {
    const artistsLinks = parseAndReplaceArtists(sourceValue, artists, className)

    if (artistsLinks.length > 0) {
      // For artist role, append remixers if available, avoiding duplicates
      if (role === 'artist' && remixers.length > 0) {
        // Track which artists are already displayed to avoid duplicates
        const displayedArtistIds = new Set(
          artists.map((artist) => artist.id).filter(Boolean),
        )

        // Only add remixers that aren't already in the artists list
        const uniqueRemixers = remixers.filter(
          (remixer) => remixer.id && !displayedArtistIds.has(remixer.id),
        )

        if (uniqueRemixers.length > 0) {
          artistsLinks.push(' • ')
          uniqueRemixers.forEach((remixer, index) => {
            if (index > 0) artistsLinks.push(' • ')
            artistsLinks.push(
              <ALink
                artist={remixer}
                className={className}
                key={`remixer-${remixer.id}`}
              />,
            )
          })
        }
      }

      return <div className={className}>{artistsLinks}</div>
    }
  }

  // Fall back to regular handling
  if (artists.length === 0 && sourceValue) {
    artists = [
      {
        name: sourceValue,
        id: record[`${source}Id`] as ArtistLink['id'],
      },
    ]
  }

  // For artist role, combine artists and remixers before deduplication
  const allArtists = role === 'artist' ? [...artists, ...remixers] : artists

  // Dedupe artists and collect subroles
  const seen = new Map<ArtistLink['id'], number>()
  const dedupedArtists: ArtistLink[] = []
  let limitedShow = false

  for (const artist of allArtists) {
    if (!artist?.id) continue

    if (!seen.has(artist.id)) {
      if (dedupedArtists.length < limit) {
        seen.set(artist.id, dedupedArtists.length)
        dedupedArtists.push({
          ...artist,
          subroles: artist.subRole ? [artist.subRole] : artist.subroles,
        })
      } else {
        limitedShow = true
      }
    } else {
      const position = seen.get(artist.id)!
      const existing = dedupedArtists[position]
      if (artist.subRole) {
        if (!existing.subroles) {
          existing.subroles = [artist.subRole]
        } else if (!existing.subroles.includes(artist.subRole)) {
          existing.subroles.push(artist.subRole)
        }
      }
    }
  }

  // Create artist links
  const artistsList: React.ReactNode[] = dedupedArtists.map((artist) => (
    <ALink artist={artist} className={className} key={artist.id} />
  ))

  if (limitedShow) {
    artistsList.push(<span key="more">...</span>)
  }

  return <>{intersperse(artistsList, ' • ')}</>
}
