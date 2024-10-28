package forwardingrules

type L4ResourcesNamer interface {
	// L4ForwardingRule returns the name of the forwarding rule for the given service and protocol.
	L4ForwardingRule(namespace, name, protocol string) string
	// L4IPv6ForwardingRule returns the name of the IPv6 forwarding rule for the given service and protocol.
	L4IPv6ForwardingRule(namespace, name, protocol string) string
}

