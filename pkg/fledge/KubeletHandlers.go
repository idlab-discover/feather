package fledge

import (
	"encoding/json"
	"fmt"
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"net/http"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	stats "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

var totalNanoCores uint64

// PID limit: cat /proc/sys/kernel/pid_max
func StatsSummary(w http.ResponseWriter, r *http.Request) {
	fmt.Println("StatsSummary")

	nodenameStr, _ := util.ExecShellCommand("hostname")
	nodename := strings.TrimSuffix(nodenameStr, "\n")

	//CPU STUFF, REFACTOR TO METHOD
	cpuStatsStr, _ := util.ExecShellCommand("mpstat 1 1 | grep 'all'")

	nProc, _ := util.ExecShellCommand("nproc")
	numCpus, _ := strconv.Atoi(strings.Trim(nProc, "\n"))

	cpuStatsLines := strings.Split(cpuStatsStr, "\n")
	//cpuStatsStr = strings.TrimSuffix(cpuStatsStr, "\n")
	cpuCats := strings.Split(reInsideWhtsp.ReplaceAllString(cpuStatsLines[0], " "), " ")
	cpuIdle, _ := strconv.ParseFloat(cpuCats[len(cpuCats)-1], 64)

	cpuNanos := uint64((100-cpuIdle)*10000000) * uint64(numCpus) //pct is already 10^2, so * 10^7, then * cores.

	//TODO: take time into account here (cpuNanos * seconds passed since last check)
	totalNanoCores += cpuNanos

	cpuStats := stats.CPUStats{
		Time:                 metav1.Now(),
		UsageNanoCores:       &cpuNanos,
		UsageCoreNanoSeconds: &totalNanoCores,
	}

	//MEM STUFF, REFACTOR TO METHOD
	memStatsStr, _ := util.ExecShellCommand("free | grep 'Mem:'")
	cats := strings.Split(reInsideWhtsp.ReplaceAllString(memStatsStr, " "), " ")
	memFree, _ := strconv.ParseUint(cats[6], 10, 64)
	memSize, _ := strconv.ParseUint(cats[1], 10, 64)

	memStatsStr, _ = util.ExecShellCommand("free | grep '+'")
	//bailout for older free versions, in which case this is more accurate for "available" memory
	if memStatsStr != "" {
		cats := strings.Split(reInsideWhtsp.ReplaceAllString(memStatsStr, " "), " ")
		memFree, _ = strconv.ParseUint(cats[2], 10, 64)
	}

	memUsed := memSize - memFree

	memStats := stats.MemoryStats{
		Time:            metav1.Now(),
		UsageBytes:      &memUsed,
		AvailableBytes:  &memFree,
		WorkingSetBytes: &memUsed,
	}

	//NETWORK STUFF, REFACTOR TO METHOD

	//ifnames: / # ip a | grep -o -E '[0-9]: [a-z0-9]*: '

	ifacesStr, _ := util.ExecShellCommand("ip a | grep -o -E '[0-9]{1,2}: [a-z0-9]*: ' | grep -o -E '[a-z0-9]{2,}'")
	ifaces := strings.Split(ifacesStr, "\n")

	//ifstats: ifconfig enp1s0f0 | grep 'bytes'
	//      RX bytes:726654708 (692.9 MiB)  TX bytes:456250038 (435.1 MiB)

	ifacesStats := []stats.InterfaceStats{}
	for _, iface := range ifaces {
		ifaceStatsStr, _ := util.ExecShellCommand("ifconfig " + iface + "| grep 'bytes'")
		fmt.Println(ifaceStatsStr)
		//TODO from here on
	}

	netStats := stats.NetworkStats{
		Time:       metav1.Now(),
		Interfaces: ifacesStats,
	}

	nodeStats := stats.NodeStats{
		NodeName:  nodename,
		StartTime: metav1.NewTime(StartTime),
		CPU:       &cpuStats,
		Memory:    &memStats,
		Network:   &netStats,
		//Fs: ,
		//Runtime: ,
		//Rlimit: ,
	}

	summary := stats.Summary{
		Node: nodeStats,
	}

	if err := json.NewEncoder(w).Encode(summary); err != nil {
		panic(err)
	}
}

func DeployPod(w http.ResponseWriter, r *http.Request) {
}

func DeletePod(w http.ResponseWriter, r *http.Request) {
}
