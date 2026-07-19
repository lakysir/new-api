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
import {
  Check,
  Copy,
  FileAudio,
  FileImage,
  Film,
  Loader2,
  Trash2,
  Upload,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { api } from '@/lib/api'

type UserAsset = {
  id: number
  filename: string
  content_type: string
  media_type: 'image' | 'video' | 'audio'
  size: number
  url: string
  created_at: number
}

type ApiEnvelope<T> = {
  success: boolean
  message: string
  data: T
}

type AssetLibraryDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

export function AssetLibraryDialog(props: AssetLibraryDialogProps) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [assets, setAssets] = useState<UserAsset[]>([])
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState(0)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [pendingDelete, setPendingDelete] = useState<UserAsset | null>(null)
  const [copiedId, setCopiedId] = useState<number | null>(null)

  useEffect(() => {
    if (!props.open) return
    setLoading(true)
    void api
      .get<ApiEnvelope<UserAsset[]>>('/api/user-assets/')
      .then((response) => {
        if (!response.data.success) throw new Error(response.data.message)
        setAssets(response.data.data ?? [])
      })
      .catch(() => undefined)
      .finally(() => setLoading(false))
  }, [props.open])

  async function uploadFile(file: File) {
    if (!['image/', 'video/', 'audio/'].some((type) => file.type.startsWith(type))) {
      toast.error(t('Only image, video, and audio files are supported'))
      return
    }
    const form = new FormData()
    form.append('file', file)
    setUploading(true)
    setUploadProgress(0)
    try {
      const response = await api.post<ApiEnvelope<UserAsset>>(
        '/api/user-assets/',
        form,
        {
          skipBusinessError: true,
          onUploadProgress: (event) => {
            if (event.total) {
              setUploadProgress(Math.round((event.loaded / event.total) * 100))
            }
          },
        }
      )
      if (!response.data.success) throw new Error(response.data.message)
      setAssets((current) => [response.data.data, ...current])
      toast.success(t('Resource uploaded'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Upload failed'))
    } finally {
      setUploading(false)
      setUploadProgress(0)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  async function copyURL(asset: UserAsset) {
    try {
      await navigator.clipboard.writeText(asset.url)
      setCopiedId(asset.id)
      window.setTimeout(() => setCopiedId(null), 1600)
      toast.success(t('URL copied'))
    } catch {
      toast.error(t('Copy failed'))
    }
  }

  async function deleteAsset(asset: UserAsset) {
    setDeletingId(asset.id)
    try {
      const response = await api.delete<ApiEnvelope<null>>(
        `/api/user-assets/${asset.id}`,
        { skipBusinessError: true }
      )
      if (!response.data.success) throw new Error(response.data.message)
      setAssets((current) => current.filter((item) => item.id !== asset.id))
      toast.success(t('Resource deleted'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Delete failed'))
    } finally {
      setDeletingId(null)
      setPendingDelete(null)
    }
  }

  let libraryContent: React.ReactNode
  if (loading) {
    libraryContent = (
      <div className='flex h-56 items-center justify-center text-muted-foreground'>
        <Loader2 className='mr-2 h-5 w-5 animate-spin' />
        {t('Loading resources...')}
      </div>
    )
  } else if (assets.length === 0) {
    libraryContent = (
      <div className='flex h-56 flex-col items-center justify-center rounded-md border border-dashed text-center'>
        <Upload className='mb-3 h-8 w-8 text-muted-foreground/60' />
        <div className='text-sm font-medium'>{t('No resources uploaded')}</div>
        <div className='mt-1 max-w-xs text-xs text-muted-foreground'>
          {t('Your uploaded media will appear here with a reusable public URL.')}
        </div>
      </div>
    )
  } else {
    libraryContent = (
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        {assets.map((asset) => (
          <div key={asset.id} className='overflow-hidden rounded-md border bg-card'>
            <div className='flex aspect-video items-center justify-center overflow-hidden bg-muted/40'>
              {asset.media_type === 'image' && (
                <img className='h-full w-full object-contain' src={asset.url} alt={asset.filename} loading='lazy' />
              )}
              {asset.media_type === 'video' && (
                <video className='h-full w-full object-contain' src={asset.url} controls preload='metadata' />
              )}
              {asset.media_type === 'audio' && (
                <div className='flex w-full flex-col items-center gap-4 px-5'>
                  <FileAudio className='h-9 w-9 text-muted-foreground' />
                  <audio className='h-9 w-full' src={asset.url} controls preload='metadata' />
                </div>
              )}
            </div>
            <div className='space-y-3 p-3'>
              <div className='flex min-w-0 items-start gap-2'>
                {asset.media_type === 'image' && <FileImage className='mt-0.5 h-4 w-4 shrink-0 text-muted-foreground' />}
                {asset.media_type === 'video' && <Film className='mt-0.5 h-4 w-4 shrink-0 text-muted-foreground' />}
                {asset.media_type === 'audio' && <FileAudio className='mt-0.5 h-4 w-4 shrink-0 text-muted-foreground' />}
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-sm font-medium' title={asset.filename}>{asset.filename}</div>
                  <div className='mt-0.5 text-xs text-muted-foreground'>
                    {new Date(asset.created_at * 1000).toLocaleString()} · {formatFileSize(asset.size)}
                  </div>
                </div>
              </div>
              <div className='flex gap-2'>
                <Button className='flex-1' size='sm' variant='outline' onClick={() => void copyURL(asset)}>
                  {copiedId === asset.id ? <Check className='mr-2 h-4 w-4' /> : <Copy className='mr-2 h-4 w-4' />}
                  {copiedId === asset.id ? t('Copied') : t('Copy URL')}
                </Button>
                <Button
                  size='icon-sm'
                  variant='ghost'
                  className='text-muted-foreground hover:text-destructive'
                  aria-label={t('Delete resource')}
                  title={t('Delete resource')}
                  disabled={deletingId === asset.id}
                  onClick={() => setPendingDelete(asset)}
                >
                  {deletingId === asset.id ? <Loader2 className='h-4 w-4 animate-spin' /> : <Trash2 className='h-4 w-4' />}
                </Button>
              </div>
            </div>
          </div>
        ))}
      </div>
    )
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='flex max-h-[88vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl'>
        <DialogHeader className='border-b px-5 py-4 text-left'>
          <DialogTitle>{t('Resource library')}</DialogTitle>
          <DialogDescription>
            {t('Upload media here, then copy its public URL into script parameters.')}
          </DialogDescription>
        </DialogHeader>

        <div className='flex items-center justify-between gap-4 border-b bg-muted/25 px-5 py-3'>
          <div className='min-w-0 text-xs text-muted-foreground'>
            {t('Images, video, and audio. Up to 100 MB per file.')}
          </div>
          <input
            ref={fileInputRef}
            className='hidden'
            type='file'
            accept='image/*,video/*,audio/*'
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) void uploadFile(file)
            }}
          />
          <Button
            type='button'
            className='shrink-0'
            disabled={uploading}
            onClick={() => fileInputRef.current?.click()}
          >
            {uploading ? (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            ) : (
              <Upload className='mr-2 h-4 w-4' />
            )}
            {uploading ? t('Uploading...') : t('Upload resource')}
          </Button>
        </div>
        {uploading && <Progress value={uploadProgress} className='h-1 rounded-none' />}

        <div className='min-h-56 flex-1 overflow-y-auto p-5'>
          {libraryContent}
        </div>
      </DialogContent>
      <AlertDialog open={pendingDelete != null} onOpenChange={(open) => { if (!open && deletingId == null) setPendingDelete(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete resource')}</AlertDialogTitle>
            <AlertDialogDescription>{t('Delete this resource permanently?')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletingId != null}>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deletingId != null}
              onClick={() => { if (pendingDelete) void deleteAsset(pendingDelete) }}
            >
              {deletingId != null && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  )
}
