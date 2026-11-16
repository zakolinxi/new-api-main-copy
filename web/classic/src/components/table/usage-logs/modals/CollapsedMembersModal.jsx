/*
Copyright (C) 2025 QuantumNous

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

import React, { useMemo } from 'react';
import { Modal, Table, Typography, Empty, Tag } from '@douyinfe/semi-ui';
import {
  getLogOther,
  renderNumber,
  renderQuota,
  timestamp2string,
} from '../../../../helpers';

const { Text } = Typography;

/**
 * 折叠行展开：显示这一行合并了哪些原始调用。
 *
 * 折叠是在「最近的日志窗口」里做的视图，所以这里取到的成员可能比行上的
 * collapsed_count 多（组内更早的调用落在窗口外），两个数字各自都是真的。
 */
const CollapsedMembersModal = ({
  visible,
  onCancel,
  target,
  members,
  total,
  loading,
  t,
}) => {
  const columns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        width: 190,
        render: (value) => (
          <Text size='small' style={{ fontVariantNumeric: 'tabular-nums' }}>
            {timestamp2string(value)}
          </Text>
        ),
      },
      {
        title: t('Tokens'),
        width: 260,
        render: (_, record) => {
          const other = getLogOther(record.other);
          const cacheRead = other?.cache_tokens || 0;
          const cacheWrite5m = other?.cache_creation_tokens_5m || 0;
          const cacheWrite1h = other?.cache_creation_tokens_1h || 0;
          const cacheWrite =
            cacheWrite5m > 0 || cacheWrite1h > 0
              ? cacheWrite5m + cacheWrite1h
              : other?.cache_creation_tokens || 0;

          // 与列表「输入」列保持一致的排版：主数字在上，缓存读写作第二行小字，
          // 不再单独占一列。
          let cacheText = '';
          if (cacheRead > 0 && cacheWrite > 0) {
            cacheText = `${t('缓存读')} ${cacheRead.toLocaleString()} · ${t('写')} ${cacheWrite.toLocaleString()}`;
          } else if (cacheRead > 0) {
            cacheText = `${t('缓存读')} ${cacheRead.toLocaleString()}`;
          } else if (cacheWrite > 0) {
            cacheText = `${t('缓存写')} ${cacheWrite.toLocaleString()}`;
          }

          return (
            <div
              style={{
                display: 'inline-flex',
                flexDirection: 'column',
                alignItems: 'flex-start',
                lineHeight: 1.2,
              }}
            >
              <span style={{ fontVariantNumeric: 'tabular-nums' }}>
                {renderNumber(record.prompt_tokens || 0)} /{' '}
                {renderNumber(record.completion_tokens || 0)}
              </span>
              {cacheText ? (
                <span
                  style={{
                    marginTop: 2,
                    fontSize: 11,
                    color: 'var(--semi-color-text-2)',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {cacheText}
                </span>
              ) : null}
            </div>
          );
        },
      },
      {
        title: t('首字延迟'),
        width: 120,
        render: (_, record) => {
          const other = getLogOther(record.other);
          const firstToken = other?.frt;
          return (
            <Text size='small' style={{ fontVariantNumeric: 'tabular-nums' }}>
              {firstToken > 0 ? `${(firstToken / 1000).toFixed(2)}s` : '-'}
            </Text>
          );
        },
      },
      {
        title: 'IP',
        dataIndex: 'ip',
        width: 160,
        render: (value) => (
          <Text size='small' type={value ? undefined : 'tertiary'}>
            {value || '-'}
          </Text>
        ),
      },
      {
        title: t('额度'),
        dataIndex: 'quota',
        width: 150,
        render: (value) => (
          <Text size='small' style={{ fontVariantNumeric: 'tabular-nums' }}>
            {renderQuota(value || 0, 6)}
          </Text>
        ),
      },
    ],
    [t],
  );

  const shown = members.length;
  const mergedCount = target?.collapsed_count || 0;

  return (
    <Modal
      title={t('合并调用明细')}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      centered
      closable
      maskClosable
      width={920}
      bodyStyle={{ padding: '0 24px 16px' }}
    >
      <div>
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            alignItems: 'center',
            gap: 8,
            marginBottom: 8,
          }}
        >
          <Tag color='blue' className='!rounded-md'>
            {target?.username || '—'} · {target?.model_name || '—'}
          </Tag>
          <Text type='tertiary' size='small'>
            {t('这一行合并了 {{merged}} 次调用，下面列出最近 {{shown}} 条。', {
              merged: mergedCount,
              shown,
            })}
          </Text>
        </div>

        {loading ? (
          <div style={{ padding: '32px 0', textAlign: 'center' }}>
            <Text type='tertiary'>{t('加载中...')}</Text>
          </div>
        ) : shown === 0 ? (
          <Empty
            description={t('没有可显示的单次调用记录')}
            style={{ padding: '24px 0' }}
          />
        ) : (
          <>
            {/* 明细可能到 50 条，把可用高度吃满再滚动，避免只露出十来行就要滚。 */}
            <div
              style={{
                maxHeight: 'calc(82vh - 170px)',
                overflowY: 'auto',
                overflowX: 'hidden',
                border: '1px solid var(--semi-color-border)',
                borderRadius: 8,
              }}
            >
              <Table
                columns={columns}
                dataSource={members}
                rowKey='id'
                pagination={false}
                size='small'
                loading={false}
                className='collapsed-members-table'
              />
            </div>
            {total > shown && (
              <div style={{ marginTop: 8 }}>
                <Text type='tertiary' size='small'>
                  {t('共 {{total}} 条，仅展示最近 {{shown}} 条。', {
                    total,
                    shown,
                  })}
                </Text>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
};

export default CollapsedMembersModal;
