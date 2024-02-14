package system

import (
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"k8s.io/apimachinery/pkg/api/resource"
)

func CpuCount() (resource.Quantity, error) {
	cpuCount, _ := util.ExecShellCommand("nproc")
	return resource.ParseQuantity(util.RemoveSpace(cpuCount))
}
