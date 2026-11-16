package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSettingReadDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	return db
}

// TestGetSettingIsReadOnly covers the destructive side effect the getter used to
// have: on a decode error it cleared the stored setting, so reading a legacy or
// hand-edited row erased the administrator's proxy, plugin bindings, and system
// prompt. Reads happen on the relay hot path, so this was reachable from
// ordinary traffic, not only from the admin UI.
func TestGetSettingIsReadOnly(t *testing.T) {
	db := setupChannelSettingReadDB(t)

	// task_plugin_key is a string, so a numeric value fails to decode.
	const corrupted = `{"task_plugin_key":123}`
	channel := &Channel{Name: "legacy", Key: "sk", Setting: stringPointer(corrupted)}
	require.NoError(t, channel.Insert())

	loaded, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 0, loaded.GetSetting().HTTP2ConnectionShards, "a corrupt setting decodes as the zero value")

	loaded, err = GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, loaded.Setting, "the getter must not clear the in-memory value")
	assert.Equal(t, corrupted, *loaded.Setting)

	var stored string
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Select("setting").Scan(&stored).Error)
	assert.Equal(t, corrupted, stored, "the getter must never rewrite the stored setting")

	// A valid setting keeps decoding normally after a corrupt one was observed.
	valid := &Channel{Name: "healthy", Key: "sk", Setting: stringPointer(`{"proxy":"http://proxy.example:8080"}`)}
	require.NoError(t, valid.Insert())
	healthy, err := GetChannelById(valid.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "http://proxy.example:8080", healthy.GetSetting().Proxy)
}

// TestGetOtherSettingsIsReadOnly mirrors the setting getter: an undecodable
// settings column is reported, not overwritten with "{}".
func TestGetOtherSettingsIsReadOnly(t *testing.T) {
	db := setupChannelSettingReadDB(t)

	const corrupted = `{"client_identity":"not-an-object"}`
	channel := &Channel{Name: "legacy-other", Key: "sk", OtherSettings: corrupted}
	require.NoError(t, channel.Insert())

	loaded, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Nil(t, loaded.GetOtherSettings().ClientIdentity)

	var stored string
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Select("settings").Scan(&stored).Error)
	assert.Equal(t, corrupted, stored, "the getter must never rewrite the stored settings")
}

func stringPointer(value string) *string {
	return &value
}
