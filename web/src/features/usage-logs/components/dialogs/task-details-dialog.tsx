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
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import type { TaskLog } from '../../types'

function hasMeaningfulTaskData(data: unknown): boolean {
  if (data == null) return false
  if (typeof data === 'string') return data.trim() !== ''
  if (Array.isArray(data)) return data.length > 0
  if (typeof data === 'object') return Object.keys(data).length > 0
  return true
}

function parseTaskDataValue(data: unknown): unknown {
  if (!hasMeaningfulTaskData(data)) return undefined
  if (typeof data !== 'string') return data
  try {
    return JSON.parse(data)
  } catch {
    return data
  }
}

function formatTaskData(data: unknown): string {
  if (!hasMeaningfulTaskData(data)) return '-'
  if (typeof data === 'string') {
    try {
      return JSON.stringify(JSON.parse(data), null, 2)
    } catch {
      return data
    }
  }
  try {
    return JSON.stringify(data, null, 2)
  } catch {
    return String(data)
  }
}

interface TaskDetailsDialogProps {
  log: TaskLog
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TaskDetailsDialog({
  log,
  open,
  onOpenChange,
}: TaskDetailsDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const taskData = formatTaskData(log.data)
  const taskDataValue = parseTaskDataValue(log.data)
  const copyText = JSON.stringify(
    {
      task_id: log.task_id,
      status: log.status,
      result_url: log.result_url,
      fail_reason: log.fail_reason,
      data: taskDataValue,
    },
    null,
    2
  )

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Log Details')}
      description={t('View the complete details for this log entry')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <ScrollArea className='max-h-[70vh] pr-4'>
        <div className='space-y-4 py-4'>
          <div className='flex items-center justify-end'>
            <Button
              variant='ghost'
              size='sm'
              className='h-8 gap-1.5 px-2'
              onClick={() => copyToClipboard(copyText)}
              title={t('Copy to clipboard')}
            >
              {copiedText === copyText ? (
                <Check className='size-4 text-green-600' />
              ) : (
                <Copy className='size-4' />
              )}
              <span>{t('Copy to clipboard')}</span>
            </Button>
          </div>

          <div className='space-y-2'>
            <Label className='text-sm font-semibold'>{t('Task ID')}</Label>
            <div className='bg-muted/40 rounded-md border p-3'>
              <p className='font-mono text-sm break-all'>{log.task_id || '-'}</p>
            </div>
          </div>

          {log.result_url ? (
            <div className='space-y-2'>
              <Label className='text-sm font-semibold'>{t('URL')}</Label>
              <div className='bg-muted/40 rounded-md border p-3'>
                <a
                  href={log.result_url}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-foreground text-sm break-all hover:underline'
                >
                  {log.result_url}
                </a>
              </div>
            </div>
          ) : null}

          {log.fail_reason ? (
            <div className='space-y-2'>
              <Label className='text-sm font-semibold'>{t('Fail Reason')}</Label>
              <div className='bg-muted/40 rounded-md border p-3'>
                <p className='text-sm leading-relaxed break-all whitespace-pre-wrap text-red-600 dark:text-red-400'>
                  {log.fail_reason}
                </p>
              </div>
            </div>
          ) : null}

          {hasMeaningfulTaskData(log.data) ? (
            <div className='space-y-2'>
              <Label className='text-sm font-semibold'>{t('Raw JSON')}</Label>
              <div className='bg-muted/40 rounded-md border p-3'>
                <pre className='overflow-x-auto font-mono text-xs leading-relaxed whitespace-pre-wrap break-all'>
                  {taskData}
                </pre>
              </div>
            </div>
          ) : null}
        </div>
      </ScrollArea>
    </Dialog>
  )
}
