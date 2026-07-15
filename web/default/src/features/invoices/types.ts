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
  profile_id: number
  title: string
  tax_number: string
  registered_address: string
  registered_phone: string
  bank_name: string
  bank_account: string
  email: string
  amount_cents: number
  remark: string
  status: InvoiceStatus
  reject_reason: string
  created_at: number
  reviewed_at: number
  sent_at: number
}

export interface InvoiceProfile {
  id: number
  title: string
  tax_number: string
  registered_address: string
  registered_phone: string
  bank_name: string
  bank_account: string
  email: string
  is_default: boolean
}

export interface InvoiceOverview {
  enabled: boolean
  paid_cents: number
  occupied_cents: number
  available_cents: number
  applications: InvoiceApplication[]
}
