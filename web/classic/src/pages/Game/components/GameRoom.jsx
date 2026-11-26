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

import React, { useEffect, useState } from 'react';
import { Button, Modal, Spin, Tag, Typography } from '@douyinfe/semi-ui';
import { IconChevronLeft } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams, Navigate } from 'react-router-dom';
import { API } from '../../../helpers';
import { isAdmin, isRoot } from '../../../helpers/utils';
import { GameTestModeContext } from './GameModal';
import useGameVisibility from './useGameVisibility';
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
import GameHelpButton from './GameHelpButton';
import '../game-paper.css';

const { Title, Text } = Typography;

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

const GAME_META = {
  texas: '德州扑克',
  roulette: '恶魔轮盘',
  snake: '贪吃蛇',
  longnight: '漫漫长夜',
  g1024: '1024 TOKEN',
  tetris: '俄罗斯 TOKEN',
  stock: 'TOKEN股市',
  futures: 'TOKEN永续合约',
  goldminer: '黄金矿工',
  mining: 'TOKEN挖矿',
  roguelike: '肉鸽 NEON-PULSE',
};

// 每个游戏的测试模式说明（说明只影响提示文案，实际加成由各游戏自己实现）
const TEST_HINTS = {
  mining:
    '矿工已满（350 个）、深度 60 米、镐子满耐久，分数从 5 万起，便于观察满层产出。',
  texas: '双方各 100 万筹码开局，盲注仍按每 6 手翻倍。',
  tetris: '从第 10 级开始（约 100ms/格），棋盘为空。',
  roulette: '玩家 5 点生命、AI 1 点，放大镜与啤酒各 9 个。',
  longnight: '直接从第 90 秒开始（最后一波），火势与生命全满。',
  g1024: '仅显示测试标识，棋盘仍需自行开局。',
  snake: '仅显示测试标识，速度由局内控制。',
  goldminer: '仅显示测试标识，关卡由局内推进。',
  stock: '仅显示测试标识：行情与持仓来自服务端，构造数据请用后台股票管理。',
  futures: '仅显示测试标识：保证金与仓位来自服务端，不下发测试资金。',
  roguelike: '仅显示测试标识：游戏在 iframe 内运行，分数由游戏上报。',
};

const GameRoom = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { gameKey } = useParams();
  const GameComponent = GAME_COMPONENTS[gameKey];
  const gameIndex = Object.keys(GAME_COMPONENTS).indexOf(gameKey) + 1;
  const gameName = t(GAME_META[gameKey] || '');
  // 游戏可以把「未兑换分数」上报上来（挖矿的分数只存在页面内存里），
  // 退出时据此拦截一次，避免手滑点返回把分数丢掉。
  const [exitGuard, setExitGuard] = useState(null);
  // 测试模式：仅管理员可见，进入后按各游戏的测试配置启动，且关闭兑换
  const [testMode, setTestMode] = useState(false);
  // 全局可见性：游戏被隐藏后普通用户不能进入，root 保留通道以便恢复
  const {
    hidden: gameHidden,
    setHidden: setGameHidden,
    saving: togglingVisibility,
  } = useGameVisibility();
  // 单游戏可见性：已隐藏/未上线的游戏普通用户直达 URL 也要回大厅
  const [accessState, setAccessState] = useState('checking');

  const canTest = isAdmin();
  const canManageVisibility = isRoot();

  useEffect(() => {
    window.scrollTo(0, 0);
    setExitGuard(null);
    setTestMode(false);
  }, [gameKey]);

  // 向服务端确认这个游戏对当前用户是否可见：
  // 已隐藏的游戏普通用户拿不到（后端过滤），未上线只允许管理员直接进入。
  useEffect(() => {
    let cancelled = false;
    setAccessState('checking');
    (async () => {
      try {
        const res = await API.get('/api/user/game/list');
        const list = res.data?.data || [];
        const target = list.find((item) => item.game_key === gameKey);
        const blocked = !target || (target.status !== 'available' && !canTest);
        if (!cancelled) setAccessState(blocked ? 'blocked' : 'ok');
      } catch (error) {
        // 接口异常不拦人，避免把整个游戏页卡死
        if (!cancelled) setAccessState('ok');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [gameKey, canTest]);

  // 游戏已对用户隐藏：非 root 直接回首页，避免手输 URL 绕过
  if (gameHidden && !canManageVisibility) {
    return <Navigate to='/' replace />;
  }

  // 这个游戏本身被隐藏/未上线：普通用户回大厅（管理员保留直达，便于调试和恢复）
  if (accessState === 'blocked') {
    return <Navigate to='/game' replace />;
  }

  // 校验期间不放行游戏组件，避免先初始化再被卸载
  if (accessState === 'checking') {
    return (
      <div className='game-paper-page game-room'>
        <main
          className='game-paper-shell text-center'
          style={{ paddingTop: 120 }}
        >
          <Spin size='large' />
        </main>
      </div>
    );
  }

  const requestTestMode = () => {
    Modal.confirm({
      title: t('进入测试模式'),
      content: (
        <div>
          <p style={{ marginBottom: 8 }}>
            {t(
              '是否进入测试模式？本局按测试配置启动，不产生真实额度，兑换入口会关闭。',
            )}
          </p>
          <p style={{ color: 'var(--semi-color-text-2)', margin: 0 }}>
            {t(TEST_HINTS[gameKey] || '本游戏暂无测试加成。')}
          </p>
        </div>
      ),
      okText: t('进入'),
      cancelText: t('取消'),
      onOk: () => setTestMode(true),
    });
  };

  const leaveRoom = () => navigate('/game');

  const handleExit = () => {
    const score = Math.floor(Number(exitGuard?.score) || 0);
    if (score <= 0) {
      leaveRoom();
      return;
    }
    Modal.confirm({
      title: t('还有分数未兑换'),
      content: t(
        '当前还有 {{n}} 分没有兑换，离开后这些分数会直接丢失（进度不保存）。建议先兑换成额度，确定现在退出吗？',
        { n: score },
      ),
      okText: t('仍然退出'),
      cancelText: t('继续游戏'),
      onOk: leaveRoom,
    });
  };

  if (!GameComponent) {
    return (
      <div className='game-paper-page game-room'>
        <main className='game-paper-shell text-center'>
          <Text className='game-eyebrow'>{t('档案不存在')}</Text>
          <Title heading={4}>{t('未知游戏')}</Title>
          <Button className='mt-4' onClick={() => navigate('/game')}>
            {t('返回游戏大厅')}
          </Button>
        </main>
      </div>
    );
  }

  return (
    <div className='game-paper-page game-room'>
      <main className='game-paper-shell'>
        <header className='game-room__header'>
          <Button
            className='game-room__back'
            icon={<IconChevronLeft />}
            type='tertiary'
            aria-label={t('返回大厅')}
            onClick={handleExit}
          />
          <div className='game-room__title'>
            <Text className='game-room__folio'>
              ARCADE FILE / {String(gameIndex).padStart(2, '0')} / {gameKey}
            </Text>
            <div className='game-room__title-row'>
              <Title heading={4} className='!m-0'>
                {gameName}
              </Title>
              <GameHelpButton gameKey={gameKey} gameName={gameName} t={t} />
            </div>
          </div>
          <div className='flex items-center gap-2'>
            {testMode && <Tag color='orange'>{t('测试模式')}</Tag>}
            {canTest && (
              <Button
                size='small'
                theme={testMode ? 'solid' : 'borderless'}
                type={testMode ? 'warning' : 'tertiary'}
                onClick={() =>
                  testMode ? setTestMode(false) : requestTestMode()
                }
              >
                {testMode ? t('退出测试') : t('进入测试')}
              </Button>
            )}
          </div>
        </header>
        {gameHidden && (
          <div
            className='mb-2 rounded-lg px-3 py-2 flex items-center gap-2 flex-wrap'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <Tag color='red' size='small'>
              {t('已对用户隐藏')}
            </Tag>
            <Text size='small' type='secondary'>
              {t(
                '顶栏入口已移除，普通用户访问游戏页会回到首页；当前仅管理员可见。',
              )}
            </Text>
            <Button
              size='small'
              theme='borderless'
              type='danger'
              loading={togglingVisibility}
              onClick={() => setGameHidden(false)}
            >
              {t('恢复显示')}
            </Button>
          </div>
        )}
        {testMode && (
          <div
            className='mb-2 rounded-lg px-3 py-2 flex items-center gap-2 flex-wrap'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <Tag color='orange' size='small'>
              {t('测试模式')}
            </Tag>
            <Text size='small' type='secondary'>
              {t(TEST_HINTS[gameKey] || '本游戏暂无测试加成。')}
            </Text>
          </div>
        )}
        <section className='game-room__stage'>
          <GameTestModeContext.Provider value={testMode}>
            <GameComponent
              key={testMode ? 'test' : 'play'}
              t={t}
              fullScreen
              testMode={testMode}
              onExitGuardChange={setExitGuard}
            />
          </GameTestModeContext.Provider>
        </section>
      </main>
    </div>
  );
};

export default GameRoom;
