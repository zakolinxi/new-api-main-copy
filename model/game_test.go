package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedeemGameScoreAwardsOneQuotaUnitPerThousandPoints(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
	})

	user := &User{Username: "game-redeem-user", Password: "password"}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "snake", Name: "贪吃蛇", Status: GameStatusAvailable, MaxScore: 100000,
	}).Error)

	quota, usd, err := RedeemGameScore(user.Id, user.Username, "snake", 1000)
	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit), quota)
	assert.Equal(t, 1.0, usd)

	userQuota, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit), userQuota)

	var scoreLog GameScoreLog
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&scoreLog).Error)
	assert.Equal(t, 1000, scoreLog.Score)
	assert.Equal(t, int(common.QuotaPerUnit), scoreLog.QuotaAwarded)
	assert.Equal(t, 1.0, scoreLog.UsdAwarded)
}

func TestInitGameConfigsRaisesLegacyMaxScore(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
	})

	// 旧版本种子把挖矿上限写成了 10 万分，前端到顶后会整笔兑换 100 万分而被拒收
	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "mining", Name: "TOKEN挖矿", Status: GameStatusAvailable, MaxScore: 100000,
	}).Error)

	require.NoError(t, InitGameConfigs())

	game, err := GetGameConfigByKey("mining")
	require.NoError(t, err)
	assert.Equal(t, 1000000, game.MaxScore)
}

func TestRedeemRoguelikeScoreAwardsOneQuotaUnitPerOneHundredThousandPoints(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
	})

	user := &User{Username: "roguelike-redeem-user", Password: "password"}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "roguelike", Name: "肉鸽 NEON-PULSE", Status: GameStatusAvailable, MaxScore: 100000,
	}).Error)

	quota, usd, err := RedeemGameScore(user.Id, user.Username, "roguelike", 100000)
	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit), quota)
	assert.Equal(t, 1.0, usd)

	userQuota, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, int(common.QuotaPerUnit), userQuota)

	var scoreLog GameScoreLog
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&scoreLog).Error)
	assert.Equal(t, "roguelike", scoreLog.GameKey)
	assert.Equal(t, 100000, scoreLog.Score)
	assert.Equal(t, int(common.QuotaPerUnit), scoreLog.QuotaAwarded)
	assert.Equal(t, 1.0, scoreLog.UsdAwarded)
}

func TestRedeemRejectsGameWithoutScoreCap(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
	})

	user := &User{Username: "stock-redeem-user", Password: "password"}
	require.NoError(t, DB.Create(user).Error)
	// 股市、合约直接交易真实额度，MaxScore 为 0，不能当成「无上限兑换」放行
	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "stock", Name: "TOKEN股市", Status: GameStatusAvailable, MaxScore: 0,
	}).Error)

	quota, usd, err := RedeemGameScore(user.Id, user.Username, "stock", 1000000)
	require.Error(t, err)
	assert.Zero(t, quota)
	assert.Zero(t, usd)

	userQuota, err := GetUserQuota(user.Id, true)
	require.NoError(t, err)
	assert.Zero(t, userQuota)
}

func TestHiddenGameCanBeToggledButNotRedeemed(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
	})

	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "snake", Name: "贪吃蛇", Status: GameStatusAvailable, MaxScore: 100000, SortOrder: 1,
	}).Error)

	// hidden 是合法状态，能写进库
	require.NoError(t, UpdateGameConfigs([]GameConfig{{
		GameKey: "snake", Name: "贪吃蛇", Status: GameStatusHidden, SortOrder: 1,
	}}))
	game, err := GetGameConfigByKey("snake")
	require.NoError(t, err)
	assert.Equal(t, GameStatusHidden, game.Status)
	assert.True(t, IsGameHidden(game.Status))

	// 非法状态仍然拒绝
	assert.Error(t, UpdateGameConfigs([]GameConfig{{
		GameKey: "snake", Name: "贪吃蛇", Status: "gone", SortOrder: 1,
	}}))

	// 隐藏后普通用户不能兑换
	user := &User{Username: "hidden-game-user", Password: "password"}
	require.NoError(t, DB.Create(user).Error)
	quota, usd, err := RedeemGameScore(user.Id, user.Username, "snake", 1000)
	assert.Error(t, err)
	assert.Zero(t, quota)
	assert.Zero(t, usd)
}

func TestListVisibleGameConfigsFiltersHiddenForUsers(t *testing.T) {
	truncateTables(t)
	// options 表用于读取页面标题，迁移上避免查询报错噪声
	require.NoError(t, DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}, &Option{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
	})

	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "snake", Name: "贪吃蛇", Status: GameStatusAvailable, SortOrder: 1,
	}).Error)
	require.NoError(t, DB.Create(&GameConfig{
		GameKey: "tetris", Name: "俄罗斯", Status: GameStatusHidden, SortOrder: 2,
	}).Error)

	// 普通用户：拿不到隐藏的游戏
	visible, err := ListVisibleGameConfigs(false)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	assert.Equal(t, "snake", visible[0].GameKey)

	// 管理员：全量返回，才能在大厅里恢复
	all, err := ListVisibleGameConfigs(true)
	require.NoError(t, err)
	require.Len(t, all, 2)
}
