import React from 'react'
import { Notification as RANotification } from 'react-admin'

const Notification = (props: React.ComponentProps<typeof RANotification>) => (
  <RANotification
    {...props}
    anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
  />
)

export default Notification
