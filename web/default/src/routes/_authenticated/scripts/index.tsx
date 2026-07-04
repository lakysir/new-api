import { createFileRoute } from '@tanstack/react-router'

import { ScriptsPage } from '@/features/scripts'

export const Route = createFileRoute('/_authenticated/scripts/')({
  component: ScriptsPage,
})
