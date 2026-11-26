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

import { useCallback, useContext, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { StatusContext } from '../../../context/Status';

// 与后端 HeaderNavModules 默认值保持一致：缺失 game 字段视为「显示」
const DEFAULT_NAV_MODULES = {
  home: true,
  console: true,
  pricing: { enabled: true, requireAuth: false },
  docs: true,
  about: true,
  game: true,
};

export function parseNavModules(raw) {
  if (!raw || typeof raw !== 'string' || raw.trim() === '') {
    return { ...DEFAULT_NAV_MODULES };
  }
  try {
    const modules = JSON.parse(raw);
    if (!modules || typeof modules !== 'object') {
      return { ...DEFAULT_NAV_MODULES };
    }
    if (typeof modules.pricing === 'boolean') {
      modules.pricing = { enabled: modules.pricing, requireAuth: false };
    }
    if (typeof modules.game !== 'boolean') {
      modules.game = true;
    }
    return modules;
  } catch (error) {
    return { ...DEFAULT_NAV_MODULES };
  }
}

export function isGameModuleHidden(raw) {
  return parseNavModules(raw).game === false;
}

/**
 * 游戏模块的全局可见性（复用顶栏模块配置里的 game 开关）。
 * 隐藏后顶栏入口消失、普通用户访问 /game 会被重定向，管理员（root）仍可进入恢复。
 */
export default function useGameVisibility() {
  const { t } = useTranslation();
  const [statusState, statusDispatch] = useContext(StatusContext);
  const [saving, setSaving] = useState(false);
  // ref 兜底：同一帧内连点两次时 state 还没更新，用 ref 挡住并发写
  const savingRef = useRef(false);

  const raw = statusState?.status?.HeaderNavModules;
  const hidden = useMemo(() => isGameModuleHidden(raw), [raw]);

  const setHidden = useCallback(
    async (nextHidden) => {
      if (savingRef.current) return false;
      savingRef.current = true;
      setSaving(true);
      try {
        const modules = parseNavModules(statusState?.status?.HeaderNavModules);
        const payload = { ...modules, game: !nextHidden };
        const res = await API.put('/api/option/', {
          key: 'HeaderNavModules',
          value: JSON.stringify(payload),
        });
        if (res.data.success) {
          statusDispatch({
            type: 'set',
            payload: {
              ...(statusState?.status || {}),
              HeaderNavModules: JSON.stringify(payload),
            },
          });
          showSuccess(
            nextHidden ? t('已对用户隐藏游戏模块') : t('已恢复显示游戏模块'),
          );
          return true;
        }
        showError(res.data.message || t('保存失败，请重试'));
      } catch (error) {
        showError(error.message || t('保存失败，请重试'));
      } finally {
        savingRef.current = false;
        setSaving(false);
      }
      return false;
    },
    [statusDispatch, statusState, t],
  );

  return { hidden, setHidden, saving };
}
