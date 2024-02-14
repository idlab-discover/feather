// base on: <https://github.com/containerd/containerd/blob/main/platforms/cpuinfo.go>
/*
   Copyright The containerd Authors.
   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at
       http://www.apache.org/licenses/LICENSE-2.0
   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package platform

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"sync"
)

// Present the ARM instruction set architecture, eg: v7, v8
// Don't use this value directly; call cpuVariant() instead.
var cpuVariantValue string

var cpuVariantOnce sync.Once

func cpuVariant() string {
	cpuVariantOnce.Do(func() {
		switch runtime.GOARCH {
		case "arm", "arm64":
			cpuVariantValue = getCPUVariant()
		}
	})
	return cpuVariantValue
}

// For Linux, the kernel has already detected the ABI, ISA and Features.
// So we don't need to access the ARM registers to detect platform information
// by ourselves. We can just parse these information from /proc/cpuinfo
func getCPUInfo(pattern string) (info string) {
	if runtime.GOOS != "linux" {
		return ""
	}

	cpuinfo, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer cpuinfo.Close()

	// Start to Parse the Cpuinfo line by line. For SMP SoC, we parse
	// the first core is enough.
	scanner := bufio.NewScanner(cpuinfo)
	for scanner.Scan() {
		newline := scanner.Text()
		list := strings.Split(newline, ":")

		if len(list) > 1 && strings.EqualFold(strings.TrimSpace(list[0]), pattern) {
			return strings.TrimSpace(list[1])
		}
	}
	return ""
}

func getCPUVariant() string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// Windows/Darwin only supports v7 for ARM32 and v8 for ARM64 and so we can use
		// runtime.GOARCH to determine the variants
		switch runtime.GOARCH {
		case "arm64":
			return "v8"
		case "arm":
			return "v7"
		}
		return ""
	}

	variant := getCPUInfo("Cpu architecture")

	// handle edge case for Raspberry Pi ARMv6 devices (which due to a kernel quirk, report "CPU architecture: 7")
	// https://www.raspberrypi.org/forums/viewtopic.php?t=12614
	if runtime.GOARCH == "arm" && variant == "7" {
		model := getCPUInfo("model name")
		if strings.HasPrefix(strings.ToLower(model), "armv6-compatible") {
			variant = "6"
		}
	}

	switch strings.ToLower(variant) {
	case "8", "aarch64":
		variant = "v8"
	case "7", "7m", "?(12)", "?(13)", "?(14)", "?(15)", "?(16)", "?(17)":
		variant = "v7"
	case "6", "6tej":
		variant = "v6"
	case "5", "5t", "5te", "5tej":
		variant = "v5"
	case "4", "4t":
		variant = "v4"
	case "3":
		variant = "v3"
	default:
		variant = ""
	}

	return variant
}
