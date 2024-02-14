package system

import (
	"fmt"
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"k8s.io/apimachinery/pkg/api/resource"
)

func StorageSize() (resource.Quantity, error) {
	return storageQuantity("SIZE")
}

func StorageUsed() (resource.Quantity, error) {
	return storageQuantity("USED")
}

func StorageAvailable() (resource.Quantity, error) {
	return storageQuantity("AVAIL")
}

func storageQuantity(field string) (resource.Quantity, error) {
	// TODO: Detect which disk is used for storage
	// Read storage information with findmnt
	command := fmt.Sprintf("findmnt / --output=%s --noheadings --raw --bytes", field)
	stdout, err := util.ExecShellCommand(command)
	if err != nil {
		return resource.Quantity{}, err
	}
	// Parse the quantity from the value
	return resource.ParseQuantity(util.RemoveSpace(stdout))
}

func IsStoragePressure() bool {
	storTotal, _ := StorageSize()
	storAvailable, _ := StorageAvailable()
	return (storAvailable.AsApproximateFloat64() / storTotal.AsApproximateFloat64()) <= 0.1
}

func IsStorageFull() bool {
	storTotal, _ := StorageSize()
	storAvailable, _ := StorageAvailable()
	return (storAvailable.AsApproximateFloat64() / storTotal.AsApproximateFloat64()) <= 0.01
}
