export type InvoiceStatus =
  | 'pending'
  | 'approved'
  | 'sent'
  | 'rejected'
  | 'cancelled'

export interface InvoiceApplication {
  id: number
  user_id: number
  username?: string
  invoice_type: 'personal' | 'enterprise'
  title: string
  tax_number: string
  email: string
  amount_cents: number
  remark: string
  status: InvoiceStatus
  reject_reason: string
  created_at: number
  reviewed_at: number
  sent_at: number
}

export interface InvoiceOverview {
  enabled: boolean
  paid_cents: number
  occupied_cents: number
  available_cents: number
  applications: InvoiceApplication[]
}
