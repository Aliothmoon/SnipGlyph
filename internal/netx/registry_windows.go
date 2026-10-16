package netx

import "golang.org/x/sys/windows/registry"

// readWinProxy reads the settings on every call: the registry read takes
// microseconds and tools like Clash toggle them at any time.
func readWinProxy() winProxy {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return winProxy{}
	}
	defer k.Close()
	var p winProxy
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		p.enabled = v != 0
	}
	p.server, _, _ = k.GetStringValue("ProxyServer")
	p.override, _, _ = k.GetStringValue("ProxyOverride")
	p.pac, _, _ = k.GetStringValue("AutoConfigURL")
	return p
}
