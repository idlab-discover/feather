package provider

import (
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"io"
	"os"
)

type ContainerLogger struct {
	file *os.File
	opts api.ContainerLogOpts
}

func NewContainerLogger(path string, opts api.ContainerLogOpts) (*ContainerLogger, error) {
	file, err := os.Open(path)
	return &ContainerLogger{file: file, opts: opts}, err
}

func (s *ContainerLogger) Read(p []byte) (n int, err error) {
	n, err = s.file.Read(p)
	if s.opts.Follow && err == io.EOF {
		n, err = 0, nil
	}
	return
}

func (s *ContainerLogger) Close() error {
	return s.file.Close()
}
