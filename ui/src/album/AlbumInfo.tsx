import Table from '@mui/material/Table'
import type { ReactNode } from 'react'
import TableBody from '@mui/material/TableBody'
import { humanize, underscore } from 'inflection'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableRow from '@mui/material/TableRow'
import {
  ArrayField,
  BooleanField,
  ChipField,
  DateField,
  FunctionField,
  SingleFieldList,
  TextField,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import {
  ArtistLinkField,
  MultiLineTextField,
  ParticipantsInfo,
  RangeField,
} from '../common'
import type { AlbumRecord } from '../types/records'

type AlbumInfoProps = {
  record?: AlbumRecord
}

const AlbumInfo = (props: AlbumInfoProps) => {
  const translate = useTranslate()
  const record = useRecordContext<AlbumRecord>(props)

  if (!record) return null

  const data: Record<string, ReactNode> = {
    name: <TextField source={'name'} />,
    libraryName: <TextField source="libraryName" />,
    albumArtist: (
      <ArtistLinkField source="albumArtist" record={record} limit={Infinity} />
    ),
    genre: (
      <ArrayField source={'genres'}>
        <SingleFieldList linkType={false}>
          <ChipField source={'name'} />
        </SingleFieldList>
      </ArrayField>
    ),
    date:
      record?.maxYear && record.maxYear === record.minYear ? (
        <TextField source={'date'} />
      ) : (
        <RangeField source={'year'} />
      ),
    originalDate:
      record?.maxOriginalYear &&
      record.maxOriginalYear === record.minOriginalYear ? (
        <TextField source={'originalDate'} />
      ) : (
        <RangeField source={'originalYear'} />
      ),
    releaseDate: <TextField source={'releaseDate'} />,
    recordLabel: (
      <FunctionField
        source={'recordLabel'}
        render={(r) => r.tags?.recordlabel?.join(', ')}
      />
    ),
    catalogNum: <TextField source={'catalogNum'} />,
    releaseType: (
      <FunctionField
        source={'releaseType'}
        render={(r) => r.tags?.releasetype?.join(', ')}
      />
    ),
    media: (
      <FunctionField
        source={'media'}
        render={(r) => r.tags?.media?.join(', ')}
      />
    ),
    grouping: (
      <FunctionField
        source={'grouping'}
        render={(r) => r.tags?.grouping?.join(', ')}
      />
    ),
    mood: (
      <FunctionField
        source={'mood'}
        render={(r) => r.tags?.mood?.join(', ')}
      />
    ),
    compilation: <BooleanField source={'compilation'} />,
    updatedAt: <DateField source={'updatedAt'} showTime />,
    comment: <MultiLineTextField source={'comment'} />,
  }

  const optionalFields = ['comment', 'genre', 'catalogNum']
  optionalFields.forEach((field) => {
    !record[field] && delete data[field]
  })

  const optionalTags = [
    'releaseType',
    'recordLabel',
    'grouping',
    'mood',
    'media',
  ]
  optionalTags.forEach((field) => {
    !record?.tags?.[field.toLowerCase()] && delete data[field]
  })

  return (
    <TableContainer>
      <Table aria-label="album details" size="small">
        <TableBody>
          {Object.keys(data).map((key) => {
            return (
              <TableRow key={`${record.id}-${key}`}>
                <TableCell component="th" scope="row" sx={{ width: '17.5%' }}>
                  {translate(`resources.album.fields.${key}`, {
                    _: humanize(underscore(key)),
                  })}
                  :
                </TableCell>
                <TableCell align="left" sx={{ whiteSpace: 'pre-line' }}>
                  {data[key]}
                </TableCell>
              </TableRow>
            )
          })}
          <ParticipantsInfo record={record} tableCellSx={{ width: '17.5%' }} />
        </TableBody>
      </Table>
    </TableContainer>
  )
}

export default AlbumInfo
