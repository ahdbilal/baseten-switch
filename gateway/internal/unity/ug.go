package unity

import (
	"context"
	"unity-gateway/gateway/internal/ugruntime"
)

// RenderClient delegates base configuration to ug. The caller retains ownership
// of isolated overlays, catalogs, and narrowly scoped rollback receipts.
func (c Connection) RenderClient(ctx context.Context, request ugruntime.Request) (ugruntime.Result, error) {
	if err := c.Validate(); err != nil {
		return ugruntime.Result{}, err
	}
	request.Workspace, request.Profile = c.Workspace, c.Profile
	return ugruntime.Render(ctx, request)
}
