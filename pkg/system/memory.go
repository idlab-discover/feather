package system

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"regexp"
	"strings"
)

func MemoryTotal() (resource.Quantity, error) {
	return memoryQuantity("MemTotal")
}

func MemoryFree() (resource.Quantity, error) {
	return memoryQuantity("MemFree")
}

func MemoryAvailable() (resource.Quantity, error) {
	return memoryQuantity("MemAvailable")
}

func memoryQuantity(field string) (resource.Quantity, error) {
	// Read memory information from /proc filesystem
	contents, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return resource.Quantity{}, err
	}
	// Parse the field from the contents
	pattern := fmt.Sprintf(`%s: +([0-9]+) ([A-Za-z])B`, field)
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return resource.Quantity{}, err
	}
	matches := expression.FindStringSubmatch(string(contents))
	// The units in the /proc filesystem must be interpreted as binary
	value := fmt.Sprintf("%s%si", matches[1], strings.ToUpper(matches[2]))
	// Parse the quantity from the value
	return resource.ParseQuantity(value)
}

func IsMemoryPressure() bool {
	memTotal, _ := MemoryTotal()
	memAvailable, _ := MemoryAvailable()
	return (memAvailable.AsApproximateFloat64() / memTotal.AsApproximateFloat64()) <= 0.1
}
