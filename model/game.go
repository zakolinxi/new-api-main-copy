package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	GameStatusAvailable  = "available"
	GameStatusUnreleased = "unreleased"
	// GameStatusHidden 表示该游戏对普通用户完全不可见（列表里不出现、直达链接回大厅），
	// 只有超级管理员仍能在管理模式下看到并恢复。
	GameStatusHidden = "hidden"

	GameScoreLogTypeRedeem = "redeem"
)

// 普通计分游戏兑换比例：1000 分 = 1 美元额度
const GameScorePerUnit = 1000.0
const GameUSDPerUnit = 1.0

// 肉鸽 NEON-PULSE 兑换比例：100000 分 = 1 美元额度
const RoguelikeScorePerUnit = 100000.0

// 每小时最多兑换次数，防止无成本刷分
const GameRedeemHourlyLimit = 100

type GameConfig struct {
	Id          int    `json:"id"`
	GameKey     string `json:"game_key" gorm:"type:varchar(64);uniqueIndex"`
	Name        string `json:"name" gorm:"type:varchar(191)"`
	Description string `json:"description" gorm:"type:varchar(512)"`
	Status      string `json:"status" gorm:"type:varchar(32);index"`
	SortOrder   int    `json:"sort_order"`
	// MaxScore 单次兑换允许的最大分数；等于 0 表示该游戏不使用积分兑换（如股市、合约）
	MaxScore    int    `json:"max_score"`
	PageTitle   string `json:"page_title" gorm:"-:all"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
}

type GameScoreLog struct {
	Id           int     `json:"id"`
	UserId       int     `json:"user_id" gorm:"index"`
	Username     string  `json:"username" gorm:"type:varchar(191);index"`
	GameKey      string  `json:"game_key" gorm:"type:varchar(64);index"`
	Score        int     `json:"score"`
	QuotaAwarded int     `json:"quota_awarded"`
	UsdAwarded   float64 `json:"usd_awarded"`
	Status       string  `json:"status" gorm:"type:varchar(32)"`
	Message      string  `json:"message" gorm:"type:text"`
	CreatedTime  int64   `json:"created_time" gorm:"bigint;index"`
}

var defaultGames = []GameConfig{
	{GameKey: "texas", Name: "德州扑克", Description: "与庄家 AI 对战的经典德州扑克。", Status: GameStatusAvailable, SortOrder: 1, MaxScore: 50000},
	{GameKey: "roulette", Name: "恶魔轮盘", Description: "生命垂于一线的刺激轮盘。", Status: GameStatusAvailable, SortOrder: 2, MaxScore: 100000},
	{GameKey: "snake", Name: "贪吃蛇", Description: "经典街机贪吃蛇，吃得越多分越高。", Status: GameStatusAvailable, SortOrder: 3, MaxScore: 100000},
	{GameKey: "longnight", Name: "漫漫长夜", Description: "在无尽黑夜中生存得更久。", Status: GameStatusAvailable, SortOrder: 4, MaxScore: 100000},
	{GameKey: "g1024", Name: "1024 TOKEN", Description: "合并代币方块的益智游戏。", Status: GameStatusAvailable, SortOrder: 5, MaxScore: 100000},
	{GameKey: "tetris", Name: "俄罗斯 TOKEN", Description: "下落的代币方块消除游戏。", Status: GameStatusAvailable, SortOrder: 6, MaxScore: 200000},
	{GameKey: "stock", Name: "TOKEN股市", Description: "完全模拟真实交易时间的股市，K线行情、买卖持仓。", Status: GameStatusAvailable, SortOrder: 7, MaxScore: 0},
	{GameKey: "futures", Name: "TOKEN永续合约", Description: "带杠杆的永续合约交易，做多做空。", Status: GameStatusAvailable, SortOrder: 8, MaxScore: 0},
	{GameKey: "goldminer", Name: "黄金矿工", Description: "钩取金块换取分数。", Status: GameStatusAvailable, SortOrder: 9, MaxScore: 100000},
	// 挖矿：前端分数上限 100 万分，到顶强制结算整笔兑换，MaxScore 需同步放开
	{GameKey: "mining", Name: "TOKEN挖矿", Description: "挂机挖矿产出 TOKEN 分数。", Status: GameStatusAvailable, SortOrder: 10, MaxScore: 1000000},
	// 肉鸽：前端跨局累计得分后按余额整笔兑换。原来的 100000 与兑换门槛相同，
	// 累计余额一旦超过 10 万分就会被拒收，导致永远无法提现；上限提到一百亿。
	{GameKey: "roguelike", Name: "肉鸽 NEON-PULSE", Description: "接入 NEON-PULSE 的排行榜挑战。", Status: GameStatusAvailable, SortOrder: 11, MaxScore: 10000000000},
}

func InitGameConfigs() error {
	if err := DB.AutoMigrate(&GameConfig{}, &GameScoreLog{}); err != nil {
		return err
	}
	for _, game := range defaultGames {
		game.UpdatedTime = common.GetTimestamp()
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&game).Error; err != nil {
			return err
		}
	}
	// 老版本种子的 MaxScore 可能比当前默认值小（挖矿曾经是 10 万分），而前端分数到顶后
	// 会按上限整笔兑换，上限低于累计分数时兑换会被判定超限，分数就永久卡在顶端无法提现。
	// 这里只把上限抬到默认值，不会覆盖管理员手动调高的配置。
	for _, game := range defaultGames {
		if game.MaxScore <= 0 {
			continue
		}
		if err := DB.Model(&GameConfig{}).
			Where("game_key = ? AND max_score < ?", game.GameKey, game.MaxScore).
			Update("max_score", game.MaxScore).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetGamePageTitle() string {
	var option Option
	if err := DB.Where(commonKeyCol+" = ?", "game_page_title").First(&option).Error; err == nil {
		if strings.TrimSpace(option.Value) != "" {
			return option.Value
		}
	}
	return "纸上游乐场"
}

func SetGamePageTitle(title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title cannot be empty")
	}
	return DB.Save(&Option{Key: "game_page_title", Value: title}).Error
}

// IsValidGameStatus 校验状态取值，避免脏值写库。
func IsValidGameStatus(status string) bool {
	switch status {
	case GameStatusAvailable, GameStatusUnreleased, GameStatusHidden:
		return true
	default:
		return false
	}
}

// IsGameHidden 该状态是否对普通用户完全隐藏。
func IsGameHidden(status string) bool {
	return status == GameStatusHidden
}

func ListGameConfigs() ([]*GameConfig, error) {
	games := make([]*GameConfig, 0)
	if err := DB.Order("sort_order asc").Find(&games).Error; err != nil {
		return nil, err
	}
	title := GetGamePageTitle()
	for _, game := range games {
		game.PageTitle = title
	}
	return games, nil
}

// ListVisibleGameConfigs 返回对当前用户可见的游戏。
// includeHidden 为 true（管理员）时包含已隐藏的游戏，否则过滤掉。
func ListVisibleGameConfigs(includeHidden bool) ([]*GameConfig, error) {
	games, err := ListGameConfigs()
	if err != nil {
		return nil, err
	}
	if includeHidden {
		return games, nil
	}
	visible := make([]*GameConfig, 0, len(games))
	for _, game := range games {
		if IsGameHidden(game.Status) {
			continue
		}
		visible = append(visible, game)
	}
	return visible, nil
}

func UpdateGameConfigs(updates []GameConfig) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			if update.GameKey == "" {
				continue
			}
			if !IsValidGameStatus(update.Status) {
				return fmt.Errorf("invalid status for game %s", update.GameKey)
			}
			if err := tx.Model(&GameConfig{}).
				Where("game_key = ?", update.GameKey).
				Updates(map[string]any{
					"name":         update.Name,
					"description":  update.Description,
					"status":       update.Status,
					"sort_order":   update.SortOrder,
					"updated_time": common.GetTimestamp(),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func GetGameConfigByKey(key string) (*GameConfig, error) {
	var game GameConfig
	if err := DB.Where("game_key = ?", key).First(&game).Error; err != nil {
		return nil, err
	}
	return &game, nil
}

// RedeemGameScore 校验并兑换游戏分数为额度。返回发放的 quota。
func RedeemGameScore(userId int, username string, gameKey string, score int) (int, float64, error) {
	if score <= 0 {
		return 0, 0, errors.New("score must be positive")
	}
	game, err := GetGameConfigByKey(gameKey)
	if err != nil {
		return 0, 0, errors.New("game not found")
	}
	if game.Status != GameStatusAvailable {
		return 0, 0, errors.New("game is not available")
	}
	// MaxScore <= 0 表示该游戏不使用积分兑换（股市、合约直接交易真实额度）。
	// 不能把 0 当成「无上限」放行，否则可以伪造任意分数无限兑换额度。
	if game.MaxScore <= 0 {
		return 0, 0, errors.New("该游戏不支持积分兑换")
	}
	if score > game.MaxScore {
		return 0, 0, fmt.Errorf("分数超过该游戏上限（单次最多 %d 分），请稍后再兑换", game.MaxScore)
	}

	now := common.GetTimestamp()
	hourAgo := now - 3600
	scorePerUnit := GameScorePerUnit
	if game.GameKey == "roguelike" {
		scorePerUnit = RoguelikeScorePerUnit
	}
	usd := float64(score) / scorePerUnit * GameUSDPerUnit
	quota := int(usd * common.QuotaPerUnit)
	if quota <= 0 {
		return 0, 0, errors.New("score too low to redeem")
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := lockForUpdate(tx).Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&GameScoreLog{}).
			Where("user_id = ? AND status = 'ok' AND created_time > ?", userId, hourAgo).
			Count(&count).Error; err != nil {
			return err
		}
		if count >= GameRedeemHourlyLimit {
			return fmt.Errorf("兑换过于频繁，每小时最多 %d 次", GameRedeemHourlyLimit)
		}
		result := tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota + ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&GameScoreLog{
			UserId:       userId,
			Username:     username,
			GameKey:      game.GameKey,
			Score:        score,
			QuotaAwarded: quota,
			UsdAwarded:   usd,
			Status:       "ok",
			CreatedTime:  now,
		}).Error
	})
	if err != nil {
		return 0, 0, err
	}
	if err := cacheIncrUserQuota(userId, int64(quota)); err != nil {
		common.SysLog("failed to increase user quota cache: " + err.Error())
	}
	return quota, usd, nil
}

func ListGameScoreLogs(userId int, limit int) ([]*GameScoreLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	logs := make([]*GameScoreLog, 0)
	query := DB.Order("id desc").Limit(limit)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	return logs, query.Find(&logs).Error
}
