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

import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  processModelsData,
  processGroupsData,
  showError,
} from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModels,
  setGroups,
) => {
  const { t } = useTranslation();

  // 用 ref 读取最新的模型/分组，避免数据加载回调随每次选择变化而重建
  const modelRef = useRef(inputs.model);
  const groupRef = useRef(inputs.group);
  // 仅让最后一次模型请求生效，避免快速切换分组时旧响应覆盖新列表
  const modelsRequestIdRef = useRef(0);

  useEffect(() => {
    modelRef.current = inputs.model;
  }, [inputs.model]);

  useEffect(() => {
    groupRef.current = inputs.group;
  }, [inputs.group]);

  const loadModels = useCallback(
    async (group) => {
      const requestId = ++modelsRequestIdRef.current;
      try {
        const url = group
          ? `${API_ENDPOINTS.USER_MODELS}?group=${encodeURIComponent(group)}`
          : API_ENDPOINTS.USER_MODELS;
        const res = await API.get(url);
        const { success, message, data } = res.data;

        if (requestId !== modelsRequestIdRef.current) return;

        if (success) {
          const { modelOptions, selectedModel } = processModelsData(
            data,
            modelRef.current,
          );
          setModels(modelOptions);

          // 分组内没有当前模型时回退到该分组第一个可用模型
          if (selectedModel && selectedModel !== modelRef.current) {
            handleInputChange('model', selectedModel);
          }
        } else {
          showError(t(message));
        }
      } catch (error) {
        if (requestId !== modelsRequestIdRef.current) return;
        showError(t('加载模型失败'));
      }
    },
    [handleInputChange, setModels, t],
  );

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get(API_ENDPOINTS.USER_GROUPS);
      const { success, message, data } = res.data;

      if (success) {
        const userGroup =
          userState?.user?.group ||
          JSON.parse(localStorage.getItem('user'))?.group;
        const groupOptions = processGroupsData(data, userGroup);
        setGroups(groupOptions);

        const hasCurrentGroup = groupOptions.some(
          (option) => option.value === groupRef.current,
        );
        if (!hasCurrentGroup) {
          handleInputChange('group', groupOptions[0]?.value || '');
        }
      } else {
        showError(t(message));
      }
    } catch (error) {
      showError(t('加载分组失败'));
    }
  }, [userState, handleInputChange, setGroups, t]);

  // 自动加载数据
  useEffect(() => {
    if (userState?.user) {
      loadGroups();
    }
  }, [userState?.user, loadGroups]);

  // 分组变化时重新加载该分组可用的模型
  useEffect(() => {
    if (userState?.user) {
      loadModels(inputs.group);
    }
  }, [userState?.user, inputs.group, loadModels]);

  return {
    loadModels,
    loadGroups,
  };
};
