import React, { useCallback, useState } from 'react'
import ReactDOM from 'react-dom'
import { Chip, Dialog } from '@mui/material'
import TableContainer from '@mui/material/TableContainer'
import Paper from '@mui/material/Paper'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableRow from '@mui/material/TableRow'
import TableCell from '@mui/material/TableCell'
import { useTranslate } from 'react-admin'
import { humanize } from 'inflection'
import { hotkeyEntries } from '../hotkeys'
import { useAppHotkey } from '../hooks/useAppHotkey'
import { DialogTitle } from './DialogTitle'
import { DialogContent } from './DialogContent'

const HelpTable = (props) => {
  const translate = useTranslate()
  return ReactDOM.createPortal(
    <Dialog {...props}>
      <DialogTitle onClose={props.onClose}>
        {translate('help.title')}
      </DialogTitle>
      <DialogContent dividers>
        <TableContainer component={Paper}>
          <Table size="small">
            <TableBody>
              {hotkeyEntries.map(({ id, name, sequence }) => {
                const description = translate(`help.hotkeys.${name}`, {
                  _: humanize(name),
                })
                return (
                  <TableRow key={id}>
                    <TableCell align="right" component="th" scope="row">
                      {description}
                    </TableCell>
                    <TableCell align="left">
                      <Chip
                        label={<kbd>{sequence}</kbd>}
                        size="small"
                        variant={'outlined'}
                      />
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </TableContainer>
      </DialogContent>
    </Dialog>,
    document.body,
  )
}

export const HelpDialog = (props) => {
  const [open, setOpen] = useState(false)

  const handleClickClose = (e) => {
    setOpen(false)
    e.stopPropagation()
  }

  useAppHotkey(
    'SHOW_HELP',
    useCallback(() => setOpen(true), []),
  )

  return (
    <>
      <HelpTable open={open} onClose={handleClickClose} />
    </>
  )
}
