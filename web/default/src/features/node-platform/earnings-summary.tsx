/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { getEarnings, getPlatformEarnings } from './api'
import { microsToDisplay } from './lib/format'
import type { EarningsSummary as EarningsSummaryData } from './types'

// EarningsSummary renders the balance + day/week/month income cards shared by the
// script author, provider, and platform views. `role` selects which ledger
// account is summarized:
//   - author:   money earned as a script provider (author payable)
//   - provider: money earned running nodes (provider payable)
//   - platform: platform service-fee revenue (admin only)
export function EarningsSummary({
  role,
  refreshKey = 0,
}: {
  role: 'author' | 'provider' | 'platform'
  refreshKey?: number
}) {
  const { t } = useTranslation()
  const [data, setData] = useState<EarningsSummaryData | null>(null)

  // The "balance" card label differs by role: the platform accrues revenue,
  // while authors and providers accumulate a withdrawable payable balance.
  const balanceLabel =
    role === 'platform' ? t('Revenue balance') : t('Payable balance')

  useEffect(() => {
    let cancelled = false
    const request =
      role === 'platform' ? getPlatformEarnings() : getEarnings(role)
    request
      .then((res) => {
        if (!cancelled) setData(res)
      })
      .catch((e) => toast.error(String((e as Error).message)))
    return () => {
      cancelled = true
    }
  }, [role, refreshKey])

  const currency = data?.currency || ''
  const cards: Array<[string, number | undefined]> = [
    [balanceLabel, data?.balance_micros],
    [t('Today'), data?.day_micros],
    [t('This week'), data?.week_micros],
    [t('This month'), data?.month_micros],
  ]

  return (
    <div className='grid grid-cols-2 gap-4 md:grid-cols-4'>
      {cards.map(([label, value]) => (
        <div key={label} className='rounded-lg border p-4'>
          <div className='text-muted-foreground text-xs'>{label}</div>
          <div className='mt-1 text-lg font-semibold'>
            {microsToDisplay(value)} {currency}
          </div>
        </div>
      ))}
    </div>
  )
}
