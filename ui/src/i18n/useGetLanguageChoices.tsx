// React Hook to get a list of all languages available. English is hardcoded
import { useGetList } from 'react-admin'

const useGetLanguageChoices = () => {
  const { data = [], isPending } = useGetList('translation', {
    pagination: { page: 1, perPage: -1 },
    sort: { field: 'name', order: 'ASC' },
    filter: {},
  })

  const choices = [{ id: 'en', name: 'English' }]
  if (!isPending) {
    data.forEach(({ id, name }) => choices.push({ id, name }))
  }
  choices.sort((a, b) => a.name.localeCompare(b.name))

  return { choices, loaded: !isPending, loading: isPending }
}

export default useGetLanguageChoices
