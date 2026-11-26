import React, { useEffect, useReducer, useRef, useState } from 'react';
import { Button, Modal, Typography, Progress } from '@douyinfe/semi-ui';
import { RedeemPanel } from './GameModal';
import { API, showError } from '../../../helpers';

const { Text } = Typography;

// TOKEN 挖矿：点击挖矿累积进度，矿脉越深产量越高，但镐子会磨损（冷却）。
// 分数达到上限时整笔结算为额度，矿工最多雇佣 350 个。
const TOKEN_MINING_GAME_KEY = 'mining';
// 与后端 game_configs.max_score 的默认种子一致；进入游戏后会以后端返回的上限为准
const DEFAULT_MAX_SCORE = 1000000;
const MAX_MINERS = 350;
const REPAIR_COST = 50;
const CLICK_SCORE = 5;
const CLICK_DURABILITY_COST = 2;
const AUTO_PROGRESS_PER_MINER = 6;
const AUTO_SCORE_PER_MINER = 2;
// 防御：单次增量最多结算的层数，避免异常数值导致死循环
const MAX_LAYERS_PER_STEP = 500;

const layerNeed = (depth) => 100 + depth * 20;
const layerBonus = (depth) => 100 + depth * 10;
const pickGain = (depth) => 10 + Math.floor(depth / 2) * 2;
const hireCost = (miners) => 200 * (miners + 1);
// 上限异常时回落到默认值，避免 Math.min(NaN, ...) 把分数变成 NaN
const scoreLimit = (maxScore) =>
  Number.isFinite(maxScore) && maxScore > 0 ? maxScore : DEFAULT_MAX_SCORE;

const initialState = {
  depth: 0,
  progress: 0,
  durability: 100,
  score: 0,
  miners: 0,
  clicks: 0,
  logs: [],
  logSeq: 0,
};

const withLog = (state, entries) => {
  let seq = state.logSeq;
  const added = entries.map((entry) => ({ ...entry, id: (seq += 1) }));
  return { logs: [...added.reverse(), ...state.logs].slice(0, 5), logSeq: seq };
};

// 推进一次挖掘：增量可能一次挖穿多层（矿工多时很常见），进度必须逐层结转，不能只结算一层。
// 累计分数由传入的上限裁剪，保证永远不超过后端允许的单次兑换上限。
const advance = (state, progressGain, scoreGain, maxScore) => {
  let depth = state.depth;
  let progress = state.progress + progressGain;
  let bonus = 0;
  let layers = 0;
  while (progress >= layerNeed(depth) && layers < MAX_LAYERS_PER_STEP) {
    progress -= layerNeed(depth);
    bonus += layerBonus(depth);
    depth += 1;
    layers += 1;
  }
  return {
    depth,
    progress,
    score: Math.min(scoreLimit(maxScore), state.score + scoreGain + bonus),
    layers,
    bonus,
  };
};

const withLayerLog = (state, result) => {
  if (!result.layers) return {};
  return withLog(state, [
    {
      text: '挖穿第 {{d}} 层！+{{n}} 分',
      params: { d: result.depth, n: result.bonus },
    },
  ]);
};

// 所有分数变动都走这一个纯函数，避免在 setState 的更新函数里再调用 setState
// （React 严格模式下更新函数会被重复执行，那样会重复加分、重复升级）。
function miningReducer(state, action) {
  switch (action.type) {
    case 'dig': {
      if (state.durability <= 0) {
        return {
          ...state,
          ...withLog(state, [{ text: '镐子坏了！用 50 分修理' }]),
        };
      }
      const result = advance(
        state,
        pickGain(state.depth),
        CLICK_SCORE,
        action.maxScore,
      );
      return {
        ...state,
        depth: result.depth,
        progress: result.progress,
        score: result.score,
        durability: Math.max(0, state.durability - CLICK_DURABILITY_COST),
        clicks: state.clicks + 1,
        ...withLayerLog(state, result),
      };
    }
    case 'auto': {
      if (state.miners <= 0) return state;
      const result = advance(
        state,
        state.miners * AUTO_PROGRESS_PER_MINER,
        state.miners * AUTO_SCORE_PER_MINER,
        action.maxScore,
      );
      return {
        ...state,
        depth: result.depth,
        progress: result.progress,
        score: result.score,
        ...withLayerLog(state, result),
      };
    }
    case 'repair': {
      if (state.score < REPAIR_COST || state.durability >= 100) return state;
      return {
        ...state,
        score: state.score - REPAIR_COST,
        durability: 100,
        ...withLog(state, [{ text: '镐子修好了' }]),
      };
    }
    case 'hire': {
      if (state.miners >= MAX_MINERS) return state;
      const cost = hireCost(state.miners);
      if (state.score < cost) return state;
      return {
        ...state,
        score: state.score - cost,
        miners: state.miners + 1,
        ...withLog(state, [
          { text: '雇佣了一名矿工（消耗 {{n}} 分）', params: { n: cost } },
        ]),
      };
    }
    // 兑换成功后按实际发放的分数扣减，结算期间新挖到的分数不会被一起抹掉
    case 'deduct': {
      const amount = Math.max(
        0,
        Math.min(state.score, Math.floor(action.amount) || 0),
      );
      return { ...state, score: state.score - amount };
    }
    case 'reset':
      return { ...initialState, logSeq: state.logSeq };
    // 测试模式：矿工拉满、深度 60 米、镐子满耐久，分数给 5 万当启动资金
    case 'test':
      return {
        ...initialState,
        logSeq: state.logSeq,
        depth: 60,
        durability: 100,
        miners: MAX_MINERS,
        score: Math.min(50000, scoreLimit(action.maxScore)),
      };
    default:
      return state;
  }
}

const TokenMining = ({ t, testMode = false, onExitGuardChange }) => {
  const [state, dispatch] = useReducer(miningReducer, initialState);
  const [maxScore, setMaxScore] = useState(DEFAULT_MAX_SCORE);
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const failedScoreRef = useRef(-1);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // 上限以后端 game_configs.max_score 为准：前端写死 100 万而后端仍是旧值时，
  // 到顶后的整笔兑换会被判超限，分数就会永久卡在顶端无法提现。
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await API.get('/api/user/game/list');
        const config = (res.data?.data || []).find(
          (game) => game.game_key === TOKEN_MINING_GAME_KEY,
        );
        const limit = Math.floor(Number(config?.max_score));
        if (!cancelled && Number.isFinite(limit) && limit > 0) {
          setMaxScore(limit);
        }
      } catch (e) {
        // 读取失败时沿用默认上限，不阻断游戏
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // 自动矿工：只在雇佣数量变化时重建定时器，产量和深度都由 reducer 内的最新状态推导
  const hasMiners = state.miners > 0;
  useEffect(() => {
    if (!hasMiners) return undefined;
    const timer = setInterval(() => {
      dispatch({ type: 'auto', maxScore });
    }, 1000);
    return () => clearInterval(timer);
  }, [hasMiners, maxScore]);

  // 分数到顶自动整笔结算：同一分数只尝试一次，失败后保留分数交给手动兑换，避免重试风暴
  useEffect(() => {
    // 测试模式的分数是虚构的，绝不能触发真实结算
    if (testMode) return;
    if (state.score < maxScore) {
      failedScoreRef.current = -1;
      return;
    }
    if (pendingRef.current) return;
    if (failedScoreRef.current === state.score) return;

    pendingRef.current = true;
    setPending(true);
    const settledScore = state.score;
    (async () => {
      try {
        const res = await API.post('/api/user/game/redeem', {
          game_key: TOKEN_MINING_GAME_KEY,
          score: settledScore,
        });
        if (res.data?.success) {
          failedScoreRef.current = -1;
          dispatch({ type: 'deduct', amount: settledScore });
          Modal.info({
            title: t('分数达到上限'),
            content: t(
              '分数达到 100 万上限，已强制结算为 {{usd}} 美元额度，可继续挖矿。',
              { usd: res.data.data?.usd?.toFixed(2) ?? '—' },
            ),
          });
        } else {
          failedScoreRef.current = settledScore;
          showError(res.data?.message || t('结算失败，可稍后手动兑换'));
        }
      } catch (e) {
        failedScoreRef.current = settledScore;
        showError(e?.response?.data?.message || e.message);
      } finally {
        pendingRef.current = false;
        if (mountedRef.current) setPending(false);
      }
    })();
  }, [state.score, maxScore, t, testMode]);

  // 测试模式：等上限取回来后再铺初始状态，避免用默认 100 万覆盖后端配置
  useEffect(() => {
    if (!testMode) return;
    dispatch({ type: 'test', maxScore });
  }, [testMode, maxScore]);

  // 挖矿的分数只活在页面内存里，退出后不会恢复。把未兑换分数上报给外层，
  // 让返回按钮在有分数时先弹一次确认，避免手滑把挖到的分数丢掉。
  useEffect(() => {
    if (!onExitGuardChange) return undefined;
    onExitGuardChange({ score: state.score });
    return () => onExitGuardChange(null);
  }, [state.score, onExitGuardChange]);

  const dig = () => dispatch({ type: 'dig', maxScore });
  const repair = () => dispatch({ type: 'repair' });

  const hire = () => dispatch({ type: 'hire' });

  const reset = () => {
    if (state.score <= 0) {
      dispatch({ type: 'reset' });
      return;
    }
    Modal.confirm({
      title: t('重新开始'),
      content: t(
        '当前 {{n}} 分还没有兑换，重新开始会清空这些分数，确定继续吗？',
        { n: state.score },
      ),
      okText: t('确定'),
      cancelText: t('取消'),
      onOk: () => dispatch({ type: 'reset' }),
    });
  };

  const need = layerNeed(state.depth);
  const progressPercent = Math.min(100, (state.progress / need) * 100);
  const minersFull = state.miners >= MAX_MINERS;
  const nextHireCost = hireCost(state.miners);

  return (
    <div>
      <div className='flex items-center justify-between mb-2 flex-wrap gap-2'>
        <Text strong>
          {t('深度')}: {state.depth}m · {t('得分')}: {state.score} / {maxScore}{' '}
          · {t('矿工')}: {state.miners}/{MAX_MINERS}
        </Text>
        <Button size='small' onClick={reset}>
          {t('重新开始')}
        </Button>
      </div>

      <div
        className='rounded-lg flex flex-col items-center justify-center py-8 cursor-pointer select-none'
        style={{ background: 'var(--semi-color-fill-0)' }}
        onClick={dig}
      >
        <div style={{ fontSize: 60 }}>⛏️</div>
        <Text strong className='mt-2'>
          {t('点击挖矿！')}
        </Text>
        <Text type='secondary' size='small'>
          {t('当前层进度')} {Math.floor(state.progress)}/{need} · {t('共点击')}{' '}
          {state.clicks} {t('次')}
        </Text>
        <div style={{ width: '80%' }} className='mt-3'>
          <Progress
            percent={progressPercent}
            showInfo={false}
            stroke='#f59e0b'
          />
        </div>
      </div>

      <div className='mt-3'>
        <Text type='secondary' size='small'>
          {t('镐子耐久')}
        </Text>
        <Progress
          percent={state.durability}
          showInfo={false}
          stroke={state.durability > 30 ? '#22c55e' : '#ef4444'}
        />
      </div>

      <div className='flex gap-2 mt-3'>
        <Button
          onClick={repair}
          disabled={state.score < REPAIR_COST || state.durability >= 100}
        >
          {t('修理镐子（50分）')}
        </Button>
        <Button
          theme='solid'
          type='primary'
          onClick={hire}
          disabled={minersFull || state.score < nextHireCost}
        >
          {minersFull
            ? t('矿工已满（{{n}}）', { n: MAX_MINERS })
            : t('雇佣矿工（{{n}}分）', { n: nextHireCost })}
        </Button>
      </div>

      {state.logs.length > 0 && (
        <div
          className='mt-3 text-xs'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          {state.logs.map((line) => (
            <div key={line.id}>{t(line.text, line.params)}</div>
          ))}
        </div>
      )}

      <Text type='secondary' className='block mt-2'>
        {t(
          '每层产出 100+深度×10 分，每次点击 +5 分。镐子会磨损，矿工自动帮你挖（上限 350 个）。分数达到 100 万将自动强制结算。',
        )}
      </Text>
      <RedeemPanel
        gameKey={TOKEN_MINING_GAME_KEY}
        score={state.score}
        onRedeemed={(amount) => dispatch({ type: 'deduct', amount })}
        disabled={pending}
        onPendingChange={(value) => {
          pendingRef.current = value;
          setPending(value);
        }}
        t={t}
      />
    </div>
  );
};

export default TokenMining;
