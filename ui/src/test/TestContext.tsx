import { useContext, useMemo, type ReactNode } from 'react'
import { Provider } from 'react-redux'
import { createStore } from 'redux'
import {
  AdminContext,
  DataProviderContext,
  type AdminContextProps,
} from 'react-admin'
import type { DataProvider } from 'ra-core'

const preserveState = (state = {}) => state
const defaultDataProvider = {
  create: () => Promise.resolve({ data: { id: 0 } }),
  delete: () => Promise.resolve({ data: { id: 0 } }),
  deleteMany: () => Promise.resolve({ data: [] }),
  getList: () => Promise.resolve({ data: [], total: 0 }),
  getMany: () => Promise.resolve({ data: [] }),
  getManyReference: () => Promise.resolve({ data: [], total: 0 }),
  getOne: () => Promise.resolve({ data: { id: 0 } }),
  update: () => Promise.resolve({ data: { id: 0 } }),
  updateMany: () => Promise.resolve({ data: [] }),
} as DataProvider

type TestContextProps = Omit<AdminContextProps, 'children' | 'dataProvider'> & {
  children: ReactNode
  dataProvider?: Partial<DataProvider> | DataProvider
  initialState?: Record<string, unknown>
}

// React-admin 5 no longer ships the old ra-test package. This compatibility
// context supplies the modern react-admin providers plus the small Redux store
// still used by Navidrome's application-specific reducers.
export const TestContext = ({
  children,
  dataProvider,
  initialState = {},
  ...adminContextProps
}: TestContextProps) => {
  const inheritedDataProvider = useContext(DataProviderContext)
  const resolvedDataProvider = useMemo(() => {
    if (dataProvider) {
      return { ...defaultDataProvider, ...dataProvider } as DataProvider
    }

    return inheritedDataProvider || defaultDataProvider
  }, [dataProvider, inheritedDataProvider])
  const reduxStore = useMemo(
    () => createStore(preserveState, initialState),
    [initialState],
  )

  return (
    <Provider store={reduxStore}>
      <AdminContext dataProvider={resolvedDataProvider} {...adminContextProps}>
        {children}
      </AdminContext>
    </Provider>
  )
}
