package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupScheduledDisableTables(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM abilities")
		DB.Exec("DELETE FROM channels")
	})
}

func createScheduledChannel(t *testing.T, name string, status int, scheduledAt int64) *Channel {
	t.Helper()
	channel := &Channel{
		Name: name, Key: "sk-test", Status: status,
		Group: "default", ScheduledDisableAt: scheduledAt,
	}
	require.NoError(t, DB.Create(channel).Error)
	return channel
}

// 到点的渠道下线；未到点、已停用的渠道不受影响。
func TestDisableDueScheduledChannelsDisablesOnlyDueChannels(t *testing.T) {
	setupScheduledDisableTables(t)
	now := common.GetTimestamp()

	due := createScheduledChannel(t, "due", common.ChannelStatusEnabled, now-60)
	future := createScheduledChannel(t, "future", common.ChannelStatusEnabled, now+3600)
	alreadyDisabled := createScheduledChannel(t, "already-disabled", common.ChannelStatusManuallyDisabled, now-60)
	noSchedule := createScheduledChannel(t, "no-schedule", common.ChannelStatusEnabled, 0)

	// 渠道被下线后，对应模型能力必须同时失效，否则路由仍会把请求发给它
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "gpt-4o", ChannelId: due.Id, Enabled: true,
	}).Error)

	disabled, err := DisableDueScheduledChannels(now, 100)
	require.NoError(t, err)
	require.Len(t, disabled, 1)
	assert.Equal(t, due.Id, disabled[0].Id)

	var afterDue Channel
	require.NoError(t, DB.First(&afterDue, due.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, afterDue.Status)
	assert.Zero(t, afterDue.ScheduledDisableAt, "下线后应清空计划，避免手动启用后又被立刻下线")

	var afterFuture Channel
	require.NoError(t, DB.First(&afterFuture, future.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, afterFuture.Status)
	assert.Equal(t, now+3600, afterFuture.ScheduledDisableAt)

	var afterDisabled Channel
	require.NoError(t, DB.First(&afterDisabled, alreadyDisabled.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, afterDisabled.Status)
	assert.Equal(t, now-60, afterDisabled.ScheduledDisableAt, "任务只处理启用中的渠道")

	var afterNoSchedule Channel
	require.NoError(t, DB.First(&afterNoSchedule, noSchedule.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, afterNoSchedule.Status)

	var ability Ability
	require.NoError(t, DB.Where("channel_id = ?", due.Id).First(&ability).Error)
	assert.False(t, ability.Enabled)

	// 同一批数据重复扫描不应再次下线
	again, err := DisableDueScheduledChannels(now, 100)
	require.NoError(t, err)
	assert.Empty(t, again)
}

// 分批处理：一次只处理 batchSize 条，调用方循环推进。
func TestDisableDueScheduledChannelsRespectsBatchSize(t *testing.T) {
	setupScheduledDisableTables(t)
	now := common.GetTimestamp()

	for i := 0; i < 3; i++ {
		createScheduledChannel(t, "due-"+string(rune('a'+i)), common.ChannelStatusEnabled, now-10)
	}

	first, err := DisableDueScheduledChannels(now, 2)
	require.NoError(t, err)
	assert.Len(t, first, 2)

	second, err := DisableDueScheduledChannels(now, 2)
	require.NoError(t, err)
	assert.Len(t, second, 1)

	third, err := DisableDueScheduledChannels(now, 2)
	require.NoError(t, err)
	assert.Empty(t, third)
}
