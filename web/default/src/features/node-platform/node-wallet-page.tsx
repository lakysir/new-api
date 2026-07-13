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

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
  estimateWithdrawalFee,
  getLedgerBalances,
  requestWithdrawal,
  simulateDeposit,
} from './api'
import { displayToMicros, microsToDisplay } from './lib/format'
import type { LedgerBalances } from './types'

export function NodeWalletPage() {
  const { t } = useTranslation()
  const [bal, setBal] = useState<LedgerBalances | null>(null)
  const [depositAmt, setDepositAmt] = useState('1')
  const [wdAmt, setWdAmt] = useState('0.5')
  const [wdAddr, setWdAddr] = useState('')
  const [wdOwner, setWdOwner] = useState<'provider' | 'author'>('provider')
  const [fee, setFee] = useState<string>('')

  async function load() {
    try {
      setBal(await getLedgerBalances())
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  useEffect(() => {
    load()
  }, [])

  async function onDeposit() {
    try {
      await simulateDeposit(displayToMicros(depositAmt), `manual-${Date.now()}`)
      toast.success(t('Deposited (simulated)'))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onEstimate() {
    try {
      const q = await estimateWithdrawalFee(displayToMicros(wdAmt))
      setFee(microsToDisplay(q.fee_micros))
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  async function onWithdraw() {
    if (!wdAddr) {
      toast.error(t('Address required'))
      return
    }
    try {
      await requestWithdrawal({
        owner_type: wdOwner,
        to_address: wdAddr,
        amount_micros: displayToMicros(wdAmt),
      })
      toast.success(t('Withdrawal submitted'))
      await load()
    } catch (e) {
      toast.error(String((e as Error).message))
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Node Wallet')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' onClick={load}>
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid grid-cols-2 gap-4 md:grid-cols-4'>
          {[
            ['Available', bal?.client_available],
            ['Reserved', bal?.client_reserved],
            ['Provider Payable', bal?.provider_payable],
            ['Author Payable', bal?.author_payable],
          ].map(([label, v]) => (
            <div key={String(label)} className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-xs'>{t(label as string)}</div>
              <div className='mt-1 text-lg font-semibold'>
                {microsToDisplay(v as number)} {bal?.currency || ''}
              </div>
            </div>
          ))}
        </div>

        <div className='mt-6 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>{t('Simulated Deposit (USD_TEST)')}</div>
          <div className='flex items-center gap-2'>
            <Input
              className='w-40'
              value={depositAmt}
              onChange={(e) => setDepositAmt(e.target.value)}
            />
            <Button onClick={onDeposit}>{t('Deposit')}</Button>
          </div>
        </div>

        <div className='mt-4 rounded-lg border p-4'>
          <div className='mb-2 text-sm font-medium'>{t('Withdraw Payable')}</div>
          <div className='flex flex-wrap items-center gap-2'>
            <select
              className='h-9 rounded-md border px-2 text-sm'
              value={wdOwner}
              onChange={(e) => setWdOwner(e.target.value as 'provider' | 'author')}
            >
              <option value='provider'>{t('Provider')}</option>
              <option value='author'>{t('Author')}</option>
            </select>
            <Input
              className='w-40'
              placeholder={t('Amount')}
              value={wdAmt}
              onChange={(e) => setWdAmt(e.target.value)}
            />
            <Input
              className='w-64'
              placeholder={t('Destination address')}
              value={wdAddr}
              onChange={(e) => setWdAddr(e.target.value)}
            />
            <Button variant='outline' onClick={onEstimate}>
              {t('Estimate fee')}
            </Button>
            <Button onClick={onWithdraw}>{t('Withdraw')}</Button>
            {fee && (
              <span className='text-muted-foreground text-sm'>
                {t('Fee')}: {fee}
              </span>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
