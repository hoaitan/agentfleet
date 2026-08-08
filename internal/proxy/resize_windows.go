//go:build windows

package proxy

func watchResize(_ int, _ agentProxy) func() {
	return func() {}
}
