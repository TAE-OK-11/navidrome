import React, { useState } from 'react'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableRow from '@mui/material/TableRow'
import {
  BooleanField,
  DateField,
  TextField,
  NumberField,
  FunctionField,
  useTranslate,
  useRecordContext,
} from 'react-admin'
import { humanize, underscore } from 'inflection'
import {
  ArtistLinkField,
  BitrateField,
  ParticipantsInfo,
  PathField,
  SizeField,
} from './index'
import { MultiLineTextField } from './MultiLineTextField'
import config from '../config'
import { AlbumLinkField } from '../song/AlbumLinkField'
import { Tab, Tabs } from '@mui/material'
import type { SongRecord } from '../types/records'

const tableCellSx = { width: '17.5%' }
const valueSx = { whiteSpace: 'pre-line' }
const gainSx = { '&:after': { content: '" db"' } }

type SongInfoProps = {
  record?: SongRecord
}

export const SongInfo = (props: SongInfoProps) => {
  const translate = useTranslate()
  const record = useRecordContext<SongRecord>(props)
  const [tab, setTab] = useState(0)

  if (!record) {
    return null
  }

  // These are already displayed in other fields or are album-level tags
  const excludedTags = [
    'genre',
    'disctotal',
    'tracktotal',
    'releasetype',
    'recordlabel',
    'media',
    'albumversion',
  ]
  const data: Record<string, React.ReactNode> = {
    path: <PathField />,
    libraryName: <TextField source="libraryName" />,
    album: (
      <AlbumLinkField source="album" sortByOrder={'ASC'} record={record} />
    ),
    discSubtitle: <TextField source="discSubtitle" />,
    albumArtist: (
      <ArtistLinkField
        source="albumArtist"
        record={record}
        limit={Infinity}
      />
    ),
    artist: (
      <ArtistLinkField source="artist" record={record} limit={Infinity} />
    ),
    genre: (
      <FunctionField
        render={(r) => r.genres?.map((g) => g.name).join(' • ')}
      />
    ),
    compilation: <BooleanField source="compilation" />,
    bitRate: <BitrateField source="bitRate" />,
    bitDepth: <NumberField source="bitDepth" />,
    sampleRate: <NumberField source="sampleRate" />,
    channels: <NumberField source="channels" />,
    size: <SizeField source="size" />,
    updatedAt: <DateField source="updatedAt" showTime />,
    playCount: <TextField source="playCount" />,
    bpm: <NumberField source="bpm" />,
    comment: <MultiLineTextField source="comment" />,
  }

  const roles: Array<[string, number]> = []

  if (record.participants) {
    for (const name of Object.keys(record.participants)) {
      if (name === 'albumartist' || name === 'artist') {
        continue
      }
      roles.push([name, record.participants[name].length])
    }
  }

  const optionalFields = [
    'discSubtitle',
    'comment',
    'bpm',
    'genre',
    'bitDepth',
    'sampleRate',
  ]
  optionalFields.forEach((field) => {
    if (!record[field]) {
      delete data[field]
    }
  })
  if ((record.playCount ?? 0) > 0) {
    data.playDate = <DateField record={record} source="playDate" showTime />
  }

  if (config.enableReplayGain) {
    data.albumGain = <NumberField source="rgAlbumGain" sx={gainSx} />
    data.trackGain = <NumberField source="rgTrackGain" sx={gainSx} />
  }

  const tags = Object.entries(record.tags ?? {}).filter(
    (tag) => !excludedTags.includes(tag[0]),
  )

  const rawTags = record.rawTags as Record<string, string[]> | undefined

  return (
    <TableContainer>
      {rawTags && (
        <Tabs value={tab} onChange={(_, value) => setTab(value)}>
          <Tab
            label={translate(`resources.song.fields.mappedTags`)}
            id="mapped-tags-tab"
            aria-controls="mapped-tags-body"
          />
          <Tab
            label={translate(`resources.song.fields.rawTags`)}
            id="raw-tags-tab"
            aria-controls="raw-tags-body"
          />
        </Tabs>
      )}
      <div
        hidden={tab === 1}
        id="mapped-tags-body"
        aria-labelledby={rawTags ? 'mapped-tags-tab' : undefined}
      >
        <Table aria-label="song details" size="small">
          <TableBody>
            {Object.keys(data).map((key) => {
              return (
                <TableRow key={`${record.id}-${key}`}>
                  <TableCell scope="row" sx={tableCellSx}>
                    {translate(`resources.song.fields.${key}`, {
                      _: humanize(underscore(key)),
                    })}
                    :
                  </TableCell>
                  <TableCell align="left" sx={valueSx}>
                    {data[key]}
                  </TableCell>
                </TableRow>
              )
            })}
            <ParticipantsInfo
              classes={undefined}
              tableCellSx={tableCellSx}
              record={record}
            />
            {tags.length > 0 && (
              <TableRow key={`${record.id}-separator`}>
                <TableCell scope="row" sx={tableCellSx}></TableCell>
                <TableCell align="left" sx={valueSx}>
                  <h4>{translate(`resources.song.fields.tags`)}</h4>
                </TableCell>
              </TableRow>
            )}
            {tags.map(([name, values]) => (
              <TableRow key={`${record.id}-tag-${name}`}>
                <TableCell scope="row" sx={tableCellSx}>
                  {name}:
                </TableCell>
                <TableCell align="left" sx={valueSx}>
                  {(values as string[]).join(' • ')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      {rawTags && (
        <div
          hidden={tab === 0}
          id="raw-tags-body"
          aria-labelledby="raw-tags-tab"
        >
          <Table size="small" aria-label="song raw tags">
            <TableBody>
              <TableRow key={`${record.id}-raw-path`}>
                <TableCell scope="row" sx={tableCellSx}>
                  {translate(`resources.song.fields.path`)}:
                </TableCell>
                <TableCell align="left">{data.path}</TableCell>
              </TableRow>
              {Object.entries(rawTags).map(([key, value]) => (
                <TableRow key={`${record.id}-raw-${key}`}>
                  <TableCell scope="row" sx={tableCellSx}>
                    {key}:
                  </TableCell>
                  <TableCell align="left" sx={valueSx}>
                    {value.join(' • ')}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </TableContainer>
  )
}
