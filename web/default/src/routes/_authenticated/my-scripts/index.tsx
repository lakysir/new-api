import { createFileRoute } from '@tanstack/react-router'

import { MyScriptsPage } from '@/features/scripts'

export const Route = createFileRoute('/_authenticated/my-scripts/')({
  component: MyScriptsPage,
})
