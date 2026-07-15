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
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
  getEarnings,
  getPlatformEarnings,
  withdrawEarnings,
  withdrawPlatformEarnings,
} from './api'
import { displayToMicros, microsToCurrency, microsToDisplay } from './lib/format'
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
  const [amount, setAmount] = useState('')
  const [withdrawing, setWithdrawing] = useState(false)

  // The "balance" card label differs by role: the platform accrues revenue,
  // while authors and providers accumulate a withdrawable payable balance.
  const balanceLabel =
    role === 'platform' ? t('Revenue balance') : t('Payable balance')

  const load = useCallback(async () => {
    const res =
      role === 'platform' ? await getPlatformEarnings() : await getEarnings(role)
    setData(res)
  }, [role])

  useEffect(() => {
    let cancelled = false
    load()
      .then(() => {
        if (cancelled) setData(null)
      })
      .catch((e) => toast.error(String((e as Error).message)))
    return () => {
      cancelled = true
    }
  }, [load, refreshKey])

  const balanceMicros = data?.balance_micros ?? 0

  // Withdraw the marketplace earnings balance into the main /wallet quota (1:1
  // in USD). Blank amount withdraws the full balance.
  async function onWithdraw() {
    const amountMicros = amount.trim()
      ? displayToMicros(amount)
      : balanceMicros
    if (amountMicros <= 0) {
      toast.error(t('Enter an amount greater than zero'))
      return
    }
    if (amountMicros > balanceMicros) {
      toast.error(t('Amount exceeds the withdrawable balance'))
      return
    }
    setWithdrawing(true)
    try {
      if (role === 'platform') {
        await withdrawPlatformEarnings(amountMicros)
      } else {
        await withdrawEarnings(role, amountMicros)
      }
      toast.success(t('Withdrawn to wallet'))
      setAmount('')
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    } finally {
      setWithdrawing(false)
    }
  }

  return (
    <div className='grid grid-cols-2 gap-4 md:grid-cols-4'>
      {/* Balance card carries the withdraw action: transfer this balance to the
          main /wallet quota shown on the wallet page. */}
      <div className='col-span-2 rounded-lg border p-4 md:col-span-1'>
        <div className='text-muted-foreground text-xs'>{balanceLabel}</div>
        <div className='mt-1 text-lg font-semibold'>
          {microsToCurrency(data?.balance_micros)}
        </div>
        <div className='mt-3 flex items-center gap-2'>
          <Input
            className='h-8 w-24'
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder={microsToDisplay(balanceMicros)}
            aria-label={t('Withdraw amount')}
          />
          <Button
            size='sm'
            className='h-8'
            disabled={withdrawing || balanceMicros <= 0}
            onClick={onWithdraw}
            title={t('Withdraw to wallet')}
          >
            {withdrawing ? t('Withdrawing...') : t('Withdraw')}
          </Button>
        </div>
        <div className='text-muted-foreground mt-2 text-xs'>
          {t('Withdraw to your wallet balance at a 1:1 rate.')}
        </div>
      </div>
      {(
        [
          [t('Today'), data?.day_micros],
          [t('This week'), data?.week_micros],
          [t('This month'), data?.month_micros],
        ] as Array<[string, number | undefined]>
      ).map(([label, value]) => (
        <div key={label} className='rounded-lg border p-4'>
          <div className='text-muted-foreground text-xs'>{label}</div>
          <div className='mt-1 text-lg font-semibold'>
            {microsToCurrency(value)}
          </div>
        </div>
      ))}
    </div>
  )
}
