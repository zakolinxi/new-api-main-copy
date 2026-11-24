import { useCallback, useRef, useState } from 'react';

// 引擎类游戏的分数是「本局累计总分」：组件里的分数只是引擎上报值的镜像。
// 兑换后如果只把组件分数清零，引擎下一次上报总分时会把已经兑换掉的分数再算一次，
// 于是同一批分数可以反复兑换（贪吃蛇、1024、黄金矿工都是这种结构）。
// 这里记录已兑换的分数，显示值 = 引擎总分 − 已兑换；引擎总分归零（重新开局）时自动重置。
export const useRedeemableScore = (initial = 0) => {
  const [score, setScore] = useState(initial);
  const redeemedRef = useRef(0);

  const sync = useCallback((total) => {
    const value = Number(total) || 0;
    if (value <= 0) {
      redeemedRef.current = 0;
      setScore(0);
      return;
    }
    setScore(Math.max(0, value - redeemedRef.current));
  }, []);

  const settle = useCallback((amount) => {
    const used = Math.max(0, Math.floor(Number(amount) || 0));
    if (used <= 0) return;
    redeemedRef.current += used;
    setScore((current) => Math.max(0, current - used));
  }, []);

  return { score, sync, settle };
};

export default useRedeemableScore;
