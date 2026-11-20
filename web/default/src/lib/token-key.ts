/**
 * 密钥前缀：与后端 `/api/status` 的 `token_key_prefix` 保持一致。
 *
 * 令牌在数据库里存的是裸 key（不带前缀），展示时才拼上前缀。前缀可以由管理员
 * 在系统设置里改成 `sk-mao-` 这类形状，所以任何拼接点都必须读这个函数，不能
 * 硬编码 `sk-`。
 *
 * 老版本后端不返回该字段，此时回落到 `sk-`。
 */
export const DEFAULT_TOKEN_KEY_PREFIX = 'sk-'

export function getTokenKeyPrefix(): string {
  if (typeof window === 'undefined') return DEFAULT_TOKEN_KEY_PREFIX
  const raw = window.localStorage.getItem('status')
  if (!raw) return DEFAULT_TOKEN_KEY_PREFIX
  try {
    const status = JSON.parse(raw) as { token_key_prefix?: string } | null
    return status?.token_key_prefix || DEFAULT_TOKEN_KEY_PREFIX
  } catch {
    return DEFAULT_TOKEN_KEY_PREFIX
  }
}

/**
 * 给裸 key 套上当前前缀；已经带前缀则原样返回，避免重复拼接。
 */
export function withTokenKeyPrefix(key: string): string {
  const trimmed = (key || '').trim()
  if (!trimmed) return ''
  const prefix = getTokenKeyPrefix()
  if (!prefix) return trimmed
  return trimmed.startsWith(prefix) ? trimmed : `${prefix}${trimmed}`
}
