package job

import (
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"time"
)

type ClientTrafficResetJob struct{}

func NewClientTrafficResetJob() *ClientTrafficResetJob { return &ClientTrafficResetJob{} }
func (j *ClientTrafficResetJob) Run() {
	var inbound service.InboundService
	restart, err := inbound.ResetDueClientTraffics(time.Now())
	if err != nil {
		logger.Warningf("Client auto traffic reset failed: %v", err)
		return
	}
	if restart {
		var core service.XrayService
		core.SetToNeedRestart()
	}
}
