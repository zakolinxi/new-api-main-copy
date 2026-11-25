package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	channelScheduledDisableTick      = 30 * time.Second
	channelScheduledDisableBatchSize = 200
)

var channelScheduledDisableOnce sync.Once

// StartChannelScheduledDisableTask 启动渠道定时下线任务：每 30 秒扫描一次到期渠道。
// 多渠道部署时只在主节点执行，避免重复下线与缓存抖动。
func StartChannelScheduledDisableTask() {
	channelScheduledDisableOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ctx := context.Background()
			logger.LogInfo(ctx, fmt.Sprintf("channel scheduled disable task started: tick=%s", channelScheduledDisableTick))
			ticker := time.NewTicker(channelScheduledDisableTick)
			defer ticker.Stop()

			runChannelScheduledDisableOnce(ctx)
			for range ticker.C {
				runChannelScheduledDisableOnce(ctx)
			}
		})
	})
}

func runChannelScheduledDisableOnce(ctx context.Context) {
	total := 0
	for {
		disabled, err := model.DisableDueScheduledChannels(common.GetTimestamp(), channelScheduledDisableBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("channel scheduled disable task failed: %v", err))
			return
		}
		for i := range disabled {
			logger.LogInfo(ctx, fmt.Sprintf("channel %d (%s) disabled by schedule at %s (%s)",
				disabled[i].Id, disabled[i].Name,
				operation_setting.FormatChannelSchedule(disabled[i].ScheduledDisableAt),
				operation_setting.ChannelScheduleTimezone))
		}
		total += len(disabled)
		if len(disabled) < channelScheduledDisableBatchSize {
			break
		}
	}
	if total > 0 {
		model.InitChannelCache()
	}
}
