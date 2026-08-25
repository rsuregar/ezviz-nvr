package pairing

import (
	"fmt"
	"net"

	"github.com/hashicorp/mdns"
)

// advertiseMDNS answers mDNS A-record queries for "nvr-agent.local" with
// this machine's LAN IP(s) while pairing mode is running, so a technician
// can open http://nvr-agent.local:<port> in a browser without first having
// to find this machine's real IP (via the log line, a router's device
// list, etc.) — the same ".local" pattern many NVR/IoT setup flows use.
//
// This only helps on networks where mDNS actually reaches across devices
// (most home/small-office LANs); some segmented/enterprise WiFi (VLANs,
// client isolation) blocks multicast entirely, in which case the IP
// candidates already printed to the log remain the fallback. Advertising a
// fixed hostname also means pairing two brand-new agents on the same LAN
// at the same time would collide on the same name — acceptable for the
// common case of onboarding one site at a time.
func advertiseMDNS(port int) (stop func(), err error) {
	noop := func() {}

	ips := make([]net.IP, 0, 4)
	for _, s := range localIPs() {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return noop, fmt.Errorf("no local IP to advertise")
	}

	service, err := mdns.NewMDNSService(
		"nvr-agent-setup",
		"_http._tcp",
		"local.",
		"nvr-agent.local.",
		port,
		ips,
		[]string{"NVR EZVIZ edge agent setup"},
	)
	if err != nil {
		return noop, err
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return noop, err
	}
	return func() { _ = server.Shutdown() }, nil
}
