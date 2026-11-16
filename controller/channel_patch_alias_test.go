package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateChannelPatchBaseDoesNotMutateOrigin covers the aliasing bug behind
// the task plugin bind permission test: the sparse PATCH base used to be a
// shallow copy of the persisted row, and decoding the request body wrote
// through the shared *string pointers. Every origin-vs-patch comparison then
// became a pointer self-compare, so an administrator holding only channel write
// could rewrite setting / base_url without channel_sensitive_write.
func TestUpdateChannelPatchBaseDoesNotMutateOrigin(t *testing.T) {
	setupTaskPluginBindChannelTest(t)

	require.True(t, authz.Can(2, common.RoleAdminUser, authz.ChannelWrite), "the admin fixture must be able to write channels")
	require.False(t, authz.Can(2, common.RoleAdminUser, authz.ChannelSensitiveWrite), "the admin fixture must lack channel_sensitive_write")

	const originSetting = `{"proxy":"http://origin.example:8080"}`

	for _, tc := range []struct {
		name      string
		userID    int
		role      int
		body      func(channelID int) string
		allowed   bool
		wantText  string
		wantProxy string
	}{
		{
			name:      "admin may rename without the sensitive permission",
			userID:    2,
			role:      common.RoleAdminUser,
			body:      func(id int) string { return fmt.Sprintf(`{"id":%d,"name":"renamed"}`, id) },
			allowed:   true,
			wantText:  `"success":true`,
			wantProxy: "http://origin.example:8080",
		},
		{
			name:   "admin may not rewrite channel settings",
			userID: 2,
			role:   common.RoleAdminUser,
			body: func(id int) string {
				return fmt.Sprintf(`{"id":%d,"setting":%q}`, id, `{"proxy":"http://attacker.example:8080"}`)
			},
			wantText:  `"success":false`,
			wantProxy: "http://origin.example:8080",
		},
		{
			name:      "admin may not repoint the upstream base url",
			userID:    2,
			role:      common.RoleAdminUser,
			body:      func(id int) string { return fmt.Sprintf(`{"id":%d,"base_url":"https://attacker.example"}`, id) },
			wantText:  `"success":false`,
			wantProxy: "http://origin.example:8080",
		},
		{
			name:      "admin may not rewrite the header override",
			userID:    2,
			role:      common.RoleAdminUser,
			body:      func(id int) string { return fmt.Sprintf(`{"id":%d,"header_override":%q}`, id, `{"X-Key":"{api_key}"}`) },
			wantText:  `"success":false`,
			wantProxy: "http://origin.example:8080",
		},
		{
			name:   "root may rewrite channel settings",
			userID: 1,
			role:   common.RoleRootUser,
			body: func(id int) string {
				return fmt.Sprintf(`{"id":%d,"setting":%q}`, id, `{"proxy":"http://root.example:8080"}`)
			},
			allowed:   true,
			wantText:  `"success":true`,
			wantProxy: "http://root.example:8080",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := "https://api.example.com"
			setting := originSetting
			channel := model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Status:  common.ChannelStatusEnabled,
				Name:    "origin",
				Models:  "gpt-4o",
				Group:   "default",
				Key:     "sk-origin",
				BaseURL: &baseURL,
				Setting: &setting,
			}
			require.NoError(t, channel.Insert())

			response := putUpdateChannel(t, tc.userID, tc.role, tc.body(channel.Id))
			assert.Contains(t, response.Body.String(), tc.wantText, response.Body.String())

			stored, err := model.GetChannelById(channel.Id, true)
			require.NoError(t, err)
			require.NotNil(t, stored.Setting)
			require.NotNil(t, stored.BaseURL)
			assert.Equal(t, "sk-origin", stored.Key, "credentials never follow an unauthorized patch")
			assert.Equal(t, tc.wantProxy, stored.GetSetting().Proxy)
			if tc.allowed {
				return
			}
			assert.Equal(t, originSetting, *stored.Setting, "a rejected patch must not persist any setting change")
			assert.Equal(t, "https://api.example.com", *stored.BaseURL, "a rejected patch must not persist any endpoint change")
		})
	}
}

// TestUpdateChannelPatchBaseKeepsStoredProxyForComparison pins the proxy
// baseline: the origin proxy must be read before the request body replaces it,
// otherwise the proxy-change side effects are computed against the new value.
func TestUpdateChannelPatchBaseKeepsStoredProxyForComparison(t *testing.T) {
	setupTaskPluginBindChannelTest(t)

	baseURL := "https://api.example.com"
	setting := `{"proxy":"http://origin.example:8080"}`
	channel := model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Status:  common.ChannelStatusEnabled,
		Name:    "proxy-origin",
		Models:  "gpt-4o",
		Group:   "default",
		Key:     "sk-origin",
		BaseURL: &baseURL,
		Setting: &setting,
	}
	require.NoError(t, channel.Insert())

	origin, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	patch := PatchChannel{Channel: *origin.DeepCopy()}
	require.NoError(t, common.Unmarshal([]byte(`{"id":1,"setting":"{\"proxy\":\"http://next.example:8080\"}"}`), &patch))

	assert.Equal(t, "http://origin.example:8080", origin.GetSetting().Proxy, "the stored row snapshot must survive request decoding")
	assert.Equal(t, "http://next.example:8080", patch.GetSetting().Proxy)
}
