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
import { useEffect, useMemo } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const sensitiveSchema = z.object({
  CheckSensitiveEnabled: z.boolean(),
  CheckSensitiveOnPromptEnabled: z.boolean(),
  SensitiveWordMatchMode: z.string(),
  SensitiveWords: z.string().optional(),
})

type SensitiveFormValues = z.infer<typeof sensitiveSchema>

type SensitiveWordsSectionProps = {
  defaultValues: SensitiveFormValues
}

const matchModes = ['keyword', 'full'] as const

// 词表按行解析：去掉首尾空白、跳过空行、按小写去重，与后端加载逻辑保持一致。
function parseWordList(text: string) {
  const words: string[] = []
  const seen = new Set<string>()
  let duplicates = 0
  for (const line of text.split('\n')) {
    const word = line.trim()
    if (!word) continue
    const key = word.toLowerCase()
    if (seen.has(key)) {
      duplicates += 1
      continue
    }
    seen.add(key)
    words.push(word)
  }
  return { words, duplicates }
}

export function SensitiveWordsSection({
  defaultValues,
}: SensitiveWordsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<SensitiveFormValues>({
    resolver: zodResolver(sensitiveSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const wordsText = form.watch('SensitiveWords') ?? ''
  const { words, duplicates } = useMemo(
    () => parseWordList(wordsText),
    [wordsText]
  )
  const matchMode = form.watch('SensitiveWordMatchMode')

  const handleCleanup = () => {
    form.setValue('SensitiveWords', words.join('\n'), { shouldDirty: true })
  }

  const onSubmit = async (values: SensitiveFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof SensitiveFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Sensitive Words')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save sensitive words'
          />
          <div className='space-y-4 lg:col-span-2'>
            <FormField
              control={form.control}
              name='CheckSensitiveEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable filtering')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Blocks messages when sensitive keywords are detected.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='CheckSensitiveOnPromptEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Inspect user prompts')}</FormLabel>
                    <FormDescription>
                      {t(
                        'When enabled, prompts are scanned before reaching upstream models.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='SensitiveWordMatchMode'
            render={({ field }) => (
              <FormItem className='lg:col-span-2'>
                <FormLabel>{t('Match Mode')}</FormLabel>
                <Select
                  items={matchModes.map((mode) => ({
                    value: mode as string,
                    label:
                      mode === 'full' ? t('Full match') : t('Keyword match'),
                  }))}
                  value={field.value}
                  onValueChange={field.onChange}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='keyword'>
                        {t('Keyword match')}
                      </SelectItem>
                      <SelectItem value='full'>{t('Full match')}</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {matchMode === 'full'
                    ? t(
                        'Blocks only when the whole text equals one of the keywords, ignoring case and surrounding whitespace. Suitable for short words that are easily triggered by longer prompts.'
                      )
                    : t(
                        'Blocks as soon as any keyword appears anywhere in the text. This is the strictest option.'
                      )}
                </FormDescription>
                {matchMode === 'full' && (
                  <p className='text-xs text-amber-600 dark:text-amber-400'>
                    {t(
                      'Full match is looser than keyword matching: as soon as the text contains anything else, the request passes. Use it only to avoid false positives from short words.'
                    )}
                  </p>
                )}
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='SensitiveWords'
            render={({ field }) => (
              <FormItem className='lg:col-span-2'>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <FormLabel>{t('Blocked keywords')}</FormLabel>
                  <div className='flex flex-wrap items-center gap-2'>
                    <Badge variant='secondary'>
                      {t('{{total}} keywords', { total: words.length })}
                    </Badge>
                    {duplicates > 0 && (
                      <Badge
                        variant='outline'
                        className='text-amber-600 dark:text-amber-400'
                      >
                        {t('{{total}} duplicates', { total: duplicates })}
                      </Badge>
                    )}
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={handleCleanup}
                      disabled={wordsText.trim() === words.join('\n')}
                    >
                      {t('Clean up list')}
                    </Button>
                  </div>
                </div>
                <FormControl>
                  <Textarea
                    rows={12}
                    placeholder={t('Enter one keyword per line')}
                    className='font-mono'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Each line is one keyword. Blank lines and duplicates (case-insensitive) are ignored, and leading or trailing whitespace is trimmed.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
