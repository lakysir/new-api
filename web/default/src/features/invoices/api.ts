import { api } from '@/lib/api'

import type {
  InvoiceApplication,
  InvoiceOverview,
  InvoiceProfile,
} from './types'

export async function getInvoiceOverview(): Promise<InvoiceOverview> {
  const res = await api.get('/api/user/invoice')
  return res.data.data
}
export async function createInvoice(data: Partial<InvoiceApplication>) {
  return (await api.post('/api/user/invoice', data)).data
}
export async function cancelInvoice(id: number) {
  return (await api.post(`/api/user/invoice/${id}/cancel`)).data
}
export async function listInvoiceProfiles(): Promise<InvoiceProfile[]> {
  const res = await api.get('/api/user/invoice/profiles')
  return Array.isArray(res.data.data) ? res.data.data : []
}
export async function saveInvoiceProfile(data: Partial<InvoiceProfile>) {
  return (await api.post('/api/user/invoice/profiles', data)).data
}
export async function deleteInvoiceProfile(id: number) {
  return (await api.delete(`/api/user/invoice/profiles/${id}`)).data
}
export async function listInvoices(status = ''): Promise<InvoiceApplication[]> {
  const res = await api.get('/api/user/invoice/applications', {
    params: { status },
  })
  return Array.isArray(res.data.data) ? res.data.data : []
}
export async function reviewInvoice(
  id: number,
  approved: boolean,
  reason = ''
) {
  return (
    await api.post(`/api/user/invoice/applications/${id}/review`, {
      approved,
      reason,
    })
  ).data
}
export async function markInvoiceSent(id: number) {
  return (await api.post(`/api/user/invoice/applications/${id}/sent`)).data
}
