package system

import (
	"gitlab.ilabt.imec.be/fledge/service/pkg/util"
	"net"
)

func ExternalIP() net.IP {
	externalIP, _ := util.ExecShellCommand("curl ifconfig.me")
	return net.ParseIP(util.RemoveSpace(externalIP))
}

func InternalIP() net.IP {
	internalIP, _ := util.ExecShellCommand("hostname -I | awk '{print $1}'")
	return net.ParseIP(util.RemoveSpace(internalIP))
}

func HostName() string {
	hostname, _ := util.ExecShellCommand("hostname")
	return util.RemoveSpace(hostname)
}

func IsNetworkAvailable() bool {
	_, err := util.ExecShellCommand("ping -c1 1.1.1.1")
	return err == nil
}

func AvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
