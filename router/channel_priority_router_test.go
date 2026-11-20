package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
)

// TestChannelPriorityRoutesUseChannelPermissions covers the two endpoints that
// back drag-to-reorder on the channel list. Reading the order is a channel
// read; rewriting it changes dispatch order for every request, so it needs the
// write permission rather than the operate one.
func TestChannelPriorityRoutesUseChannelPermissions(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/priority/order", authz.ChannelRead, controller.GetChannelPriorityOrder)
	assertChannelRoutePermission(t, http.MethodPost, "/priority/reorder", authz.ChannelWrite, controller.ReorderChannelPriority)
}
