package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWantsCollapsedLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		query    string
		wantOn   bool
		wantScan int
	}{
		{query: "", wantOn: false},
		{query: "collapse=0", wantOn: false},
		{query: "collapse=1", wantOn: true, wantScan: model.LogCollapseDefaultScanLimit},
		{query: "collapse=true", wantOn: true, wantScan: model.LogCollapseDefaultScanLimit},
		{query: "collapse=1&collapse_scan=500", wantOn: true, wantScan: 500},
		{query: "collapse=1&collapse_scan=5", wantOn: true, wantScan: model.LogCollapseMinScanLimit},
		{query: "collapse=1&collapse_scan=999999", wantOn: true, wantScan: model.LogCollapseMaxScanLimit},
	}

	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/log?"+tc.query, nil)

		enabled, scanLimit := wantsCollapsedLogs(context)
		assert.Equal(t, tc.wantOn, enabled, tc.query)
		assert.Equal(t, tc.wantScan, scanLimit, tc.query)
	}
}
