package fledge

import (
	"fmt"
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
)

func GetCgroup(namespace string, podname string, container string) string {
	cgName := fmt.Sprintf("vkubelet/%s-%s-%s", namespace, podname, container)
	return cgName
}

func CreateCgroupIfNotExists(namespace string, podname string, container string) string {
	cgName := GetCgroup(namespace, podname, container)
	if !CgroupExists(cgName) {
		CreateCgroup(cgName)
	}
	return cgName
}

func CreateCgroup(cgName string) {
	//cmd := fmt.Sprintf("cgcreate -g memory,cpu:vkubelet/%s", cgName)
	cmd := fmt.Sprintf("mkdir -p /sys/fs/cgroup/memory/%s", cgName)
	util.ExecShellCommand(cmd)
	cmd = fmt.Sprintf("mkdir -p /sys/fs/cgroup/cpu/%s", cgName)
	util.ExecShellCommand(cmd)
}

func CgroupExists(cgName string) bool {
	//cmd := fmt.Sprintf("cgget -g memory:vkubelet/%s", cgName)
	cmd := fmt.Sprintf("cat /sys/fs/cgroup/memory/%s/memory.limit_in_bytes", cgName)
	_, err := util.ExecShellCommand(cmd)
	return err == nil
}

func DeleteCgroup(cgName string) {
	//cmd := fmt.Sprintf("cgdelete memory,cpu:vkubelet/%s", cgName)
	cmd := fmt.Sprintf("rmdir /sys/fs/cgroup/memory/%s", cgName)
	util.ExecShellCommand(cmd)
	cmd = fmt.Sprintf("rmdir /sys/fs/cgroup/cpu/%s", cgName)
	util.ExecShellCommand(cmd)
}

func SetMemoryLimit(cgName string, limit int64) {
	cmd := fmt.Sprintf("echo %d > /sys/fs/cgroup/memory/%s/memory.limit_in_bytes", limit, cgName)
	//cmd := fmt.Sprintf("cgset -r memory.limit_in_bytes=%d %s", limit, cgName)
	util.ExecShellCommand(cmd)
}

func SetCpuLimit(cgName string, cpus float64) {
	//cpu.cfs_period_us=100000
	//cpu.cfs_quota=100000 * cpus?
	cmd := fmt.Sprintf("echo %d > /sys/fs/cgroup/cpu/%s/cpu.cfs_period_us", 100000, cgName)
	//cmd := fmt.Sprintf("cgset -r cpu.cfs_period_us=%d %s", 100000, cgName)
	util.ExecShellCommand(cmd)
	cmd = fmt.Sprintf("echo %d > /sys/fs/cgroup/cpu/%s/cpu.cfs_quota_us", int64(100000*cpus), cgName)
	//cmd = fmt.Sprintf("cgset -r cpu.cfs_quota_us=%d %s", int64(100000*cpus), cgName)
	util.ExecShellCommand(cmd)
}
