package glance

import (
	"testing"

	"go.uber.org/goleak"
)

type goleakTestMain struct {
	m *testing.M
}

func (m goleakTestMain) Run() int {
	exitCode := m.m.Run()
	defaultHTTPTransport.CloseIdleConnections()
	defaultInsecureHTTPTransport.CloseIdleConnections()
	return exitCode
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(goleakTestMain{m: m})
}
