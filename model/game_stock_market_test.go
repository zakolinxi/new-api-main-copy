package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 2026-11-23 是周一，10:30 属于正常交易时段
var tradingMorning = time.Date(2026, 11, 23, 10, 30, 0, 0, time.Local)

func setupStockMarketTables(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(
		&GameStock{}, &GameStockKline{}, &GameStockEvent{},
		&GameStockPosition{}, &GameStockTrade{}, &Option{},
	))
	// 关掉随机分红/拆股，避免价格断言被随机事件打乱
	corporateActionsEnabled = false
	t.Cleanup(func() {
		corporateActionsEnabled = true
		DB.Exec("DELETE FROM game_stock_klines")
		DB.Exec("DELETE FROM game_stock_events")
		DB.Exec("DELETE FROM game_stock_positions")
		DB.Exec("DELETE FROM game_stock_trades")
		DB.Exec("DELETE FROM game_stocks")
	})
}

func createMarketTestStock(t *testing.T, code string) *GameStock {
	t.Helper()
	stock := &GameStock{
		Code: code, Name: code,
		PrevClose: 100, LastPrice: 100, OpenPrice: 100,
		HighPrice: 100, LowPrice: 100, ListingPrice: 100,
	}
	require.NoError(t, DB.Create(stock).Error)
	return stock
}

func countStockKlines(t *testing.T, stockId int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, DB.Model(&GameStockKline{}).Where("stock_id = ?", stockId).Count(&n).Error)
	return n
}

// 行情引擎必须在交易时段写出K线：之前 AdvanceGameStockTick 没有任何调用点，股市是死盘。
func TestAdvanceGameStockTickWritesKlineDuringTradingSession(t *testing.T) {
	setupStockMarketTables(t)
	stock := createMarketTestStock(t, "MKT1")

	advanceGameStockTickAt(tradingMorning)
	require.Equal(t, int64(1), countStockKlines(t, stock.Id))

	var kline GameStockKline
	require.NoError(t, DB.Where("stock_id = ?", stock.Id).First(&kline).Error)
	// 当天首根K线的开盘价 = 昨收 + 0~1% 小幅高开，不能低开
	assert.GreaterOrEqual(t, kline.Open, 100.0)
	assert.LessOrEqual(t, kline.Open, 101.01)
	// 单根K线涨跌幅受 ±10% 涨跌停限制
	assert.GreaterOrEqual(t, kline.Close, 90.0)
	assert.LessOrEqual(t, kline.Close, 110.0)
	assert.GreaterOrEqual(t, kline.High, kline.Close)
	assert.LessOrEqual(t, kline.Low, kline.Close)
	assert.GreaterOrEqual(t, kline.High, kline.Open)
	assert.Positive(t, kline.Volume)

	var after GameStock
	require.NoError(t, DB.First(&after, stock.Id).Error)
	assert.InDelta(t, kline.Close, after.LastPrice, 0.0001, "最新价应跟随收盘价")
}

// 20 秒一次的 tick 在同一分钟内会跑 3 次，不能写出重复K线
func TestAdvanceGameStockTickIsIdempotentWithinSameMinute(t *testing.T) {
	setupStockMarketTables(t)
	stock := createMarketTestStock(t, "MKT2")

	for i := 0; i < 3; i++ {
		advanceGameStockTickAt(tradingMorning)
	}
	assert.Equal(t, int64(1), countStockKlines(t, stock.Id), "同一分钟只应有一根K线")

	advanceGameStockTickAt(tradingMorning.Add(time.Minute))
	assert.Equal(t, int64(2), countStockKlines(t, stock.Id))
}

// 非交易时段不得产生K线，否则行情会在夜里和周末乱走
func TestAdvanceGameStockTickSkipsClosedSessions(t *testing.T) {
	setupStockMarketTables(t)
	stock := createMarketTestStock(t, "MKT3")

	closedTimes := map[string]time.Time{
		"开盘前":     time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local),
		"集合竞价":    time.Date(2026, 11, 23, 9, 20, 0, 0, time.Local),
		"午间休市":    time.Date(2026, 11, 23, 12, 0, 0, 0, time.Local),
		"收盘后":     time.Date(2026, 11, 23, 21, 0, 0, 0, time.Local),
		"周六":      time.Date(2026, 11, 21, 10, 30, 0, 0, time.Local),
		"周日":      time.Date(2026, 11, 22, 10, 30, 0, 0, time.Local),
		"尾盘15:00": time.Date(2026, 11, 23, 15, 0, 0, 0, time.Local),
	}
	for name, ts := range closedTimes {
		advanceGameStockTickAt(ts)
		assert.Zero(t, countStockKlines(t, stock.Id), "%s 不应产生K线", name)
	}

	// 下午开盘后恢复交易
	advanceGameStockTickAt(time.Date(2026, 11, 23, 13, 0, 0, 0, time.Local))
	assert.Equal(t, int64(1), countStockKlines(t, stock.Id))
}

// 退市股票不再生成K线，价格冻结在收购价
func TestAdvanceGameStockTickSkipsDelistedStock(t *testing.T) {
	setupStockMarketTables(t)
	stock := createMarketTestStock(t, "MKT4")
	require.NoError(t, DB.Model(&GameStock{}).Where("id = ?", stock.Id).
		Update("delisted", true).Error)

	advanceGameStockTickAt(tradingMorning)
	assert.Zero(t, countStockKlines(t, stock.Id))

	var after GameStock
	require.NoError(t, DB.First(&after, stock.Id).Error)
	assert.Equal(t, 100.0, after.LastPrice, "退市后价格应锁定在收购价")
}

// 拆股必须逐根缩放历史K线，不能把所有K线抹成同一个价格
func TestApplyStockSplitScalesKlinesProportionally(t *testing.T) {
	setupStockMarketTables(t)
	stock := createMarketTestStock(t, "MKT5")

	closes := []float64{100, 110, 120}
	for i, closePrice := range closes {
		require.NoError(t, DB.Create(&GameStockKline{
			StockId: stock.Id,
			Ts:      tradingMorning.Unix() + int64(i*60),
			Open:    closePrice - 1,
			High:    closePrice + 1,
			Low:     closePrice - 2,
			Close:   closePrice,
			Volume:  1000,
		}).Error)
	}

	applyStockSplit(stock, 2)

	var klines []GameStockKline
	require.NoError(t, DB.Where("stock_id = ?", stock.Id).Order("ts asc").Find(&klines).Error)
	require.Len(t, klines, 3)
	// 2:1 拆股 → 价格减半，且相对高低关系保持
	assert.InDelta(t, 50, klines[0].Close, 0.001)
	assert.InDelta(t, 55, klines[1].Close, 0.001)
	assert.InDelta(t, 60, klines[2].Close, 0.001)
	assert.InDelta(t, 49.5, klines[0].Open, 0.001)
	assert.InDelta(t, 50.5, klines[0].High, 0.001)
	assert.InDelta(t, 49, klines[0].Low, 0.001)
	assert.NotEqual(t, klines[0].Close, klines[2].Close, "拆股后历史价格不应被抹平")
}

// 只剩 2 只股票在交易时禁止继续退市，否则股市会被清空且不可逆
func TestTriggerCorporateActionsKeepsAtLeastTwoListedStocks(t *testing.T) {
	setupStockMarketTables(t)
	createMarketTestStock(t, "MKT6")
	createMarketTestStock(t, "MKT7")

	for i := 0; i < 200; i++ {
		TriggerCorporateActions(tradingMorning)
	}

	var listed int64
	require.NoError(t, DB.Model(&GameStock{}).Where("delisted = ?", false).Count(&listed).Error)
	assert.GreaterOrEqual(t, listed, int64(2), "退市是永久操作，不能把股市清空")
}

// 全新部署必须建出所有游戏相关表（迁移入口之前漏调，全新库会直接报错）
func TestInitGameConfigsCreatesAllTablesAndSeeds(t *testing.T) {
	truncateTables(t)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM game_score_logs")
		DB.Exec("DELETE FROM game_configs")
		DB.Exec("DELETE FROM game_stock_events")
		DB.Exec("DELETE FROM game_stocks")
	})

	require.NoError(t, InitGameConfigs())
	require.NoError(t, InitGameStocks())

	game, err := GetGameConfigByKey("mining")
	require.NoError(t, err)
	assert.Equal(t, 1000000, game.MaxScore, "挖矿单次上限应为 100 万")

	// 股市/合约直接交易真实额度，不能给积分兑换开口子
	for _, key := range []string{"stock", "futures"} {
		g, err := GetGameConfigByKey(key)
		require.NoError(t, err)
		assert.Zero(t, g.MaxScore, "%s 不应允许积分兑换", key)
	}

	stocks, err := ListGameStocks()
	require.NoError(t, err)
	assert.Len(t, stocks, 5)
}
