package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// DisableDueScheduledChannels 把已到达定时下线时间且仍在启用的渠道置为手动停用。
//
// 用「手动停用」而不是「自动停用」是有意为之：自动停用会被被动恢复测试重新拉起来，
// 而定时下线是管理员明确要求的结果，必须一直保持到管理员手动启用。
// 处理成功后会清空 scheduled_disable_at，因此重新启用不会再次触发同一份计划。
func DisableDueScheduledChannels(now int64, batchSize int) ([]Channel, error) {
	if batchSize <= 0 {
		batchSize = 200
	}
	var due []Channel
	if err := DB.Select("id", "name", "scheduled_disable_at").
		Where("status = ? AND scheduled_disable_at > 0 AND scheduled_disable_at <= ?",
			common.ChannelStatusEnabled, now).
		Order("scheduled_disable_at asc").
		Limit(batchSize).
		Find(&due).Error; err != nil {
		return nil, err
	}
	if len(due) == 0 {
		return nil, nil
	}

	disabled := make([]Channel, 0, len(due))
	for i := range due {
		// 条件更新兜底并发：同一时刻管理员可能刚好手动启用了该渠道或改了计划时间，
		// 此时 RowsAffected 为 0，不应把渠道误下线。
		result := DB.Model(&Channel{}).
			Where("id = ? AND status = ? AND scheduled_disable_at > 0 AND scheduled_disable_at <= ?",
				due[i].Id, common.ChannelStatusEnabled, now).
			Updates(map[string]any{
				"status":               common.ChannelStatusManuallyDisabled,
				"scheduled_disable_at": 0,
				"auto_price_guard_id":  int64(0),
				"config_revision":      gorm.Expr("config_revision + 1"),
			})
		if result.Error != nil {
			return disabled, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := DB.Model(&Ability{}).Where("channel_id = ?", due[i].Id).Update("enabled", false).Error; err != nil {
			return disabled, err
		}
		disabled = append(disabled, due[i])
	}
	return disabled, nil
}
