package job

import (
	"sync"

	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

var clientIPJobMu sync.Mutex

// CheckClientIpJob publishes limits and refreshes live connection statistics.
// Enforcement is atomic inside the bundled core, independent of scan timing.
type CheckClientIpJob struct{}

func NewCheckClientIpJob() *CheckClientIpJob { return &CheckClientIpJob{} }

func (j *CheckClientIpJob) Run() {
	if !clientIPJobMu.TryLock() {
		return
	}
	defer clientIPJobMu.Unlock()
	if err := service.WriteClientIPLimits(); err != nil {
		logger.Warningf("[LimitIP] Failed to publish limits: %v", err)
	}
	var inboundService service.InboundService
	if err := inboundService.RefreshClientIPs(); err != nil {
		logger.Warningf("[LimitIP] Live IP tracking unavailable: %v", err)
	}
}
