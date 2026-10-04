# Relay-free local transport experiment

`go test -v -count=1 -timeout=90s ./...`

Two real tsnet nodes use a local test control plane and a STUN-only region.
There is no DERP server, no public relay map and no media proxy in control.
The test transfers an HTTP body through userspace WireGuard and asserts a direct
disco endpoint with neither DERP nor a peer relay.

This proves local direct connectivity only. It does not prove hole punching
across two NATs, CGNAT, network changes, mobile lifecycle, household isolation or
secure enrollment. `testcontrol` is deliberately not in any product build.
Do not deploy this directory. The production transport decision remains gated.
