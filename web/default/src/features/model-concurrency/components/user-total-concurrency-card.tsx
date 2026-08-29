/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

import { MODEL_CONCURRENCY_BLOCKED, type UserAsyncConcurrencyRule } from '../types'
import { UserRuleForm } from './user-rule-form'

type Props = {
  rules: UserAsyncConcurrencyRule[]
  isLoading: boolean
  pending: boolean
  deletePending: boolean
  onSave: (userId: number, maxConcurrency: number) => void
  onDelete: (userId: number) => void
}

export function UserTotalConcurrencyCard({ rules, isLoading, pending, deletePending, onSave, onDelete }: Props) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('User total async concurrency')}</CardTitle>
        <CardDescription>{t('Limits the total number of async tasks a user can have in progress across all models. 0 means unlimited.')}</CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <UserRuleForm disabled={pending} onSubmit={onSave} />
        <Table>
          <TableHeader><TableRow><TableHead>{t('User')}</TableHead><TableHead>{t('Concurrency limit')}</TableHead><TableHead>{t('In progress')}</TableHead><TableHead className='text-right'>{t('Actions')}</TableHead></TableRow></TableHeader>
          <TableBody>
            {rules.length === 0 && <TableRow><TableCell colSpan={4} className='text-muted-foreground text-center'>{isLoading ? t('Loading...') : t('No rules yet')}</TableCell></TableRow>}
            {rules.map((rule) => <TableRow key={rule.id}>
              <TableCell>{rule.username || `#${rule.user_id}`}</TableCell>
              <TableCell>{rule.max_concurrency === MODEL_CONCURRENCY_BLOCKED ? <span className='text-destructive'>{t('Not allowed')}</span> : rule.max_concurrency === 0 ? t('Unlimited') : rule.max_concurrency}</TableCell>
              <TableCell>{rule.current}</TableCell>
              <TableCell className='text-right'><Button type='button' variant='outline' size='sm' disabled={deletePending} onClick={() => onDelete(rule.user_id)}>{t('Delete')}</Button></TableCell>
            </TableRow>)}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
