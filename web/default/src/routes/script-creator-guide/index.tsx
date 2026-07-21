import { createFileRoute } from '@tanstack/react-router'

import { ScriptCreatorGuidePage } from '@/features/scripts/creator-guide-page'

export const Route = createFileRoute('/script-creator-guide/')({
  component: ScriptCreatorGuidePage,
})
