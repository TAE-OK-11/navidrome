import withStyles from '@mui/styles/withStyles'
import MuiDialogContent from '@mui/material/DialogContent'

export const DialogContent = withStyles((theme) => ({
  root: {
    padding: theme.spacing(2),
  },
}))(MuiDialogContent)
