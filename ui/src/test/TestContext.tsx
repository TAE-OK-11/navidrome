import { useContext, useMemo } from 'react'
import { Provider } from 'react-redux'
import { createStore } from 'redux'
import { AdminContext, DataProviderContext } from 'react-admin'

const preserveState = (state = {}) => state
const defaultDataProvider = {
  create: () => Promise.resolve({ data: {} }),
  delete: () => Promise.resolve({ data: {} }),
  deleteMany: () => Promise.resolve({ data: [] }),
  getList: () => Promise.resolve({ data: [], total: 0 }),
  getMany: () => Promise.resolve({ data: [] }),
  getManyReference: () => Promise.resolve({ data: [], total: 0 }),
  getOne: () => Promise.resolve({ data: {} }),
  update: () => Promise.resolve({ data: {} }),
  updateMany: () => Promise.resolve({ data: [] }),
}

// React-admin 5 no longer ships the old ra-test package. This compatibility
// context supplies the modern react-admin providers plus the small Redux store
// still used by Navidrome's application-specific reducers.
export const TestContext = ({
  children,
  dataProvider,
  initialState = {},
  ...adminContextProps
}) => {
  const inheritedDataProvider = useContext(DataProviderContext)
  const reduxStore = useMemo(
    () => createStore(preserveState, initialState),
    [initialState],
  )

  return (
    <Provider store={reduxStore}>
      <AdminContext
        dataProvider={
          dataProvider || inheritedDataProvider || defaultDataProvider
        }
        {...adminContextProps}
      >
        {children}
      </AdminContext>
    </Provider>
  )
}
