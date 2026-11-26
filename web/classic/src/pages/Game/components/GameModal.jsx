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

import React, {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import {
  SideSheet,
  Button,
  Space,
  Typography,
  InputNumber,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';

const { Text, Title } = Typography;

import TexasHoldem from './TexasHoldem';
import Roulette from './Roulette';
import SnakeGame from './SnakeGame';
import LongNight from './LongNight';
import Game1024 from './Game1024';
import TetrisToken from './TetrisToken';
import StockMarket from './StockMarket';
import FuturesTrading from './FuturesTrading';
import GoldMiner from './GoldMiner';
import TokenMining from './TokenMining';
import RoguelikeBoard from './RoguelikeBoard';

const GAME_COMPONENTS = {
  texas: TexasHoldem,
  roulette: Roulette,
  snake: SnakeGame,
  longnight: LongNight,
  g1024: Game1024,
  tetris: TetrisToken,
  stock: StockMarket,
  futures: FuturesTrading,
  goldminer: GoldMiner,
  mining: TokenMining,
  roguelike: RoguelikeBoard,
};

// 兑换面板：普通计分游戏 1000 分 = 1 额度，肉鸽 100000 分 = 1 额度
// disabled 用于外部锁定（例如挖矿正在强制结算），onPendingChange 用于告知外部兑换正在进行，
// 避免同一批分数被手动兑换和自动结算各领一次。
// onRedeemed 会带上本次实际兑换掉的分数，调用方要按这个值扣分而不是直接清零：
// 累计分数超过后端单次上限时会拆成多次请求。
const MAX_REDEEM_CHUNKS = 20;

// 测试模式由 GameRoom 提供：测试局不产生真实额度，兑换面板直接不渲染。
// 用 context 下发，避免每个游戏都手动往下传一层 testMode。
export const GameTestModeContext = createContext(false);

export const RedeemPanel = ({
  gameKey,
  score,
  onRedeemed,
  t,
  disabled = false,
  onPendingChange,
}) => {
  const [redeeming, setRedeeming] = useState(false);
  const testMode = useContext(GameTestModeContext);
  const [maxPerRedeem, setMaxPerRedeem] = useState(0);
  const sendingRef = useRef(false);
  const scorePerUnit = gameKey === 'roguelike' ? 100000 : 1000;
  const usd = (score / scorePerUnit).toFixed(4);
  const blocked = disabled || redeeming;

  // 后端 game_configs.max_score 是「单次请求」上限。累计分数超过它时整笔提交会被
  // 判为超限而永久无法提现，所以这里先取回上限，再按上限分批提交。
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get('/api/user/game/list');
        const config = (res.data?.data || []).find(
          (game) => game.game_key === gameKey,
        );
        const limit = Math.floor(Number(config?.max_score));
        if (!cancelled && Number.isFinite(limit) && limit > 0) {
          setMaxPerRedeem(limit);
        }
      } catch (e) {
        // 读不到上限时按整笔提交
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [gameKey]);

  const redeem = async () => {
    let remaining = Math.floor(score);
    if (remaining <= 0 || blocked || sendingRef.current) return;
    sendingRef.current = true;
    setRedeeming(true);
    onPendingChange?.(true);

    const perRequest = maxPerRedeem > 0 ? maxPerRedeem : remaining;
    let credited = 0;
    let totalUsd = 0;
    let chunks = 0;
    let failure = '';

    while (remaining > 0 && chunks < MAX_REDEEM_CHUNKS) {
      const amount = Math.min(remaining, perRequest);
      try {
        const res = await API.post('/api/user/game/redeem', {
          game_key: gameKey,
          score: amount,
        });
        if (!res.data.success) {
          failure = res.data.message || '';
          break;
        }
        credited += amount;
        totalUsd += res.data.data?.usd || 0;
        remaining -= amount;
        chunks += 1;
      } catch (e) {
        failure = e.message;
        break;
      }
    }

    if (credited > 0) {
      showSuccess(
        t('兑换成功：获得 {{usd}} 美元额度', { usd: totalUsd.toFixed(4) }),
      );
      onRedeemed?.(credited);
    }
    if (failure) {
      showError(failure);
    } else if (remaining > 0) {
      showError(
        t('单次最多提交 {{n}} 批，剩余 {{left}} 分可再次兑换', {
          n: MAX_REDEEM_CHUNKS,
          left: remaining,
        }),
      );
    }

    sendingRef.current = false;
    setRedeeming(false);
    onPendingChange?.(false);
  };

  const batchCount =
    maxPerRedeem > 0 ? Math.ceil(Math.floor(score) / maxPerRedeem) : 1;

  // 测试模式不给兑换入口：分数是测试数据，换成额度就是凭空发钱
  if (testMode) return null;

  return (
    <div className='game-redeem-panel'>
      <div>
        <Text strong className='game-redeem-panel__score'>
          {t('当前得分')}：{Math.floor(score)}
        </Text>
        <Text type='secondary' className='block'>
          {t('可兑换')} ≈ ${usd}（{scorePerUnit} {t('分')} = 1 {t('额度')}）
        </Text>
        {batchCount > 1 && (
          <Text type='secondary' size='small' className='block'>
            {t('单次最多兑换 {{limit}} 分，将自动分 {{n}} 批提交', {
              limit: maxPerRedeem,
              n: batchCount,
            })}
          </Text>
        )}
      </div>
      <Button
        theme='solid'
        type='primary'
        loading={redeeming}
        disabled={blocked || score < scorePerUnit}
        onClick={redeem}
      >
        {score < scorePerUnit
          ? t('满 {{score}} 分可兑换', { score: scorePerUnit })
          : t('兑换为额度')}
      </Button>
    </div>
  );
};

const GameModal = ({ game, onClose, t }) => {
  if (!game) return null;
  const GameComponent = GAME_COMPONENTS[game.game_key];

  return (
    <SideSheet
      className='game-paper-sheet'
      title={
        <Space>
          <Title heading={5} className='!m-0'>
            {game.name}
          </Title>
        </Space>
      }
      visible={!!game}
      onCancel={onClose}
      placement='right'
      width={720}
      bodyStyle={{ padding: 16, background: 'var(--semi-color-bg-0)' }}
      footer={null}
      headerExtraContent={
        <Button type='tertiary' onClick={onClose}>
          {t('关闭')}
        </Button>
      }
    >
      {GameComponent ? (
        <GameComponent t={t} />
      ) : (
        <Text type='secondary'>{t('游戏加载失败')}</Text>
      )}
    </SideSheet>
  );
};

export default GameModal;
