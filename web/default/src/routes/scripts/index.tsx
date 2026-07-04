import { createFileRoute } from '@tanstack/react-router'

import { ScriptSquarePage } from '@/features/scripts'

export const Route = createFileRoute('/scripts/')({
  component: ScriptSquarePage,
})
