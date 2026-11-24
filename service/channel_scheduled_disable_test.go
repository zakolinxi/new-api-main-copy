package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 定时下线时间必须能通过渠道编辑接口写入，并且「只改计划时间」也算配置变更，
// 否则 UpdateChannelTx 会因为快照相同直接返回、字段根本不落库。
func TestChannelAdminPatchPersistsScheduledDisableAt(t *testing.T) {
	db := setupChannelMutationDB(t, true)
	enableMutationSync(t, db)
	created, err := CreateChannels([]model.Channel{{
		Key: "secret", Models: "a", Group: "default", Status: common.ChannelStatusEnabled,
	}}, ChannelMutationTriggerCreate)
	require.NoError(t, err)

	planned := common.GetTimestamp() + 86400
	_, err = UpdateChannelAdminPatch(ChannelAdminPatchInput{
		ChannelID:        created[0].Id,
		ExpectedRevision: created[0].ConfigRevision,
		Patch:            model.Channel{Name: created[0].Name, ScheduledDisableAt: planned},
		Fields:           map[string]bool{"scheduled_disable_at": true},
		Trigger:          ChannelMutationTriggerUpdate,
	})
	require.NoError(t, err)

	var stored model.Channel
	require.NoError(t, db.First(&stored, created[0].Id).Error)
	assert.Equal(t, planned, stored.ScheduledDisableAt)

	// 显式提交 0 = 取消计划，必须真的清空（不能用「零值即忽略」的语义）
	_, err = UpdateChannelAdminPatch(ChannelAdminPatchInput{
		ChannelID:        created[0].Id,
		ExpectedRevision: stored.ConfigRevision,
		Patch:            model.Channel{Name: stored.Name},
		Fields:           map[string]bool{"scheduled_disable_at": true},
		Trigger:          ChannelMutationTriggerUpdate,
	})
	require.NoError(t, err)

	require.NoError(t, db.First(&stored, created[0].Id).Error)
	assert.Zero(t, stored.ScheduledDisableAt)
}

// 手动启用要顺带取消定时下线，否则下一轮扫描会把它重新关掉。
func TestManualEnableClearsScheduledDisableAt(t *testing.T) {
	db := setupChannelMutationDB(t, false)
	created, err := CreateChannels([]model.Channel{{
		Key: "secret", Models: "a", Group: "default", Status: common.ChannelStatusManuallyDisabled,
	}}, ChannelMutationTriggerCreate)
	require.NoError(t, err)

	planned := common.GetTimestamp() + 3600
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", created[0].Id).
		Update("scheduled_disable_at", planned).Error)

	_, err = UpdateChannelStatuses([]int{created[0].Id}, common.ChannelStatusEnabled)
	require.NoError(t, err)

	var stored model.Channel
	require.NoError(t, db.First(&stored, created[0].Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Zero(t, stored.ScheduledDisableAt)

	// 渠道本来就是启用状态时，再点一次「启用」也应视为取消计划
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", created[0].Id).
		Update("scheduled_disable_at", planned).Error)
	_, err = UpdateChannelStatuses([]int{created[0].Id}, common.ChannelStatusEnabled)
	require.NoError(t, err)
	require.NoError(t, db.First(&stored, created[0].Id).Error)
	assert.Zero(t, stored.ScheduledDisableAt)
}
