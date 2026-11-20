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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Modal, Spin, Toast, Typography } from '@douyinfe/semi-ui';
import { IconArrowDown, IconArrowUp } from '@douyinfe/semi-icons';
import { API } from '../../../../helpers';

/**
 * 渠道优先级排序。
 *
 * 后端 /api/channel/priority/order 返回的是 id 升序的原始数组，这里按
 * “priority 降序、再按 id 降序”重排成界面顺序，与列表默认排序保持一致。
 * 每次移动都带上 revision 做乐观锁，冲突时提示刷新，避免两个人同时拖动
 * 把彼此的改动覆盖掉。
 */
const ChannelPriorityModal = ({
  visible,
  onCancel,
  channels = [],
  t = (key) => key,
  onReordered,
}) => {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [order, setOrder] = useState(null);

  const nameOf = useMemo(() => {
    const map = new Map();
    (channels || []).forEach((channel) => {
      map.set(channel.id, channel.name || `#${channel.id}`);
    });
    return map;
  }, [channels]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/channel/priority/order');
      if (res.data?.success) {
        setOrder(res.data.data);
      } else {
        Toast.error(res.data?.message || t('读取渠道优先级失败'));
      }
    } catch (error) {
      Toast.error(t('读取渠道优先级失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (visible) {
      load();
    }
  }, [visible, load]);

  const sorted = useMemo(() => {
    const items = order?.items || [];
    return [...items].sort((left, right) => {
      const leftPriority = left.priority ?? 0;
      const rightPriority = right.priority ?? 0;
      if (leftPriority !== rightPriority) {
        return rightPriority - leftPriority;
      }
      return right.id - left.id;
    });
  }, [order]);

  const move = async (index, delta) => {
    const current = sorted[index];
    const target = sorted[index + delta];
    if (!current || !target || !order) return;
    setSaving(true);
    try {
      const res = await API.post('/api/channel/priority/reorder', {
        revision: order.revision,
        id: current.id,
        target_id: target.id,
        position: delta < 0 ? 'before' : 'after',
        preview: false,
      });
      if (res.data?.success) {
        setOrder(res.data.data);
        if (onReordered) onReordered();
      } else {
        Toast.error(res.data?.message || t('调整顺序失败'));
        // 冲突时重新拉一次，让界面回到真实状态。
        load();
      }
    } catch (error) {
      Toast.error(t('调整顺序失败'));
      load();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={t('渠道优先级排序')}
      visible={visible}
      onCancel={onCancel}
      footer={
        <Button onClick={onCancel}>{t('关闭')}</Button>
      }
      width={560}
    >
      <Typography.Text type='tertiary' size='small'>
        {t(
          '列表从上到下即实际选路顺序：越靠前越优先。调整后立即生效，无需重启。',
        )}
      </Typography.Text>
      <div style={{ marginTop: 12, maxHeight: 420, overflowY: 'auto' }}>
        <Spin spinning={loading}>
          {sorted.length === 0 && !loading ? (
            <div style={{ padding: 24, textAlign: 'center' }}>
              <Typography.Text type='tertiary'>
                {t('暂无渠道')}
              </Typography.Text>
            </div>
          ) : null}
          {sorted.map((item, index) => (
            <div
              key={item.id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '6px 4px',
                borderBottom: '1px solid var(--semi-color-border)',
              }}
            >
              <span
                style={{
                  width: 28,
                  color: 'var(--semi-color-text-2)',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {index + 1}
              </span>
              <span
                style={{
                  flex: 1,
                  minWidth: 0,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {nameOf.get(item.id) || `#${item.id}`}
              </span>
              <Typography.Text type='tertiary' size='small'>
                {item.priority ?? 0}
              </Typography.Text>
              <Button
                size='small'
                theme='borderless'
                icon={<IconArrowUp />}
                disabled={index === 0 || saving}
                onClick={() => move(index, -1)}
              />
              <Button
                size='small'
                theme='borderless'
                icon={<IconArrowDown />}
                disabled={index === sorted.length - 1 || saving}
                onClick={() => move(index, 1)}
              />
            </div>
          ))}
        </Spin>
      </div>
      {order?.changes?.length ? (
        <Typography.Text type='tertiary' size='small'>
          {t('上一次调整改动了')} {order.changes.length} {t('个渠道')}
        </Typography.Text>
      ) : null}
    </Modal>
  );
};

export default ChannelPriorityModal;
