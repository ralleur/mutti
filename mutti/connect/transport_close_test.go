// SPDX-License-Identifier: MPL-2.0
package connect

import (
	"net"
	"testing"
	"time"
)

// SCTP's graceful reset does not necessarily wake a read if its peer vanished.
type gracefulResetOnly struct{ net.Conn }

func (gracefulResetOnly) Close() error { return nil }

func TestChannelCloseWakesBlockedIOWithoutPeer(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "write"}[write], func(t *testing.T) {
			local, remote := net.Pipe()
			defer local.Close()
			defer remote.Close()
			c := &channelConn{DataChannel: gracefulResetOnly{local}}
			done := make(chan error, 1)
			go func() {
				if write {
					_, err := c.Write([]byte("blocked"))
					done <- err
				} else {
					_, err := c.Read(make([]byte, 8))
					done <- err
				}
			}()
			// Give the I/O a chance to block; the test also accepts Close winning
			// the scheduling race, since subsequent I/O must remain closed.
			time.Sleep(20 * time.Millisecond)
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			_ = c.SetDeadline(time.Time{}) // A late TLS deadline reset must not reopen I/O.
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("blocked I/O succeeded after close")
				}
			case <-time.After(time.Second):
				t.Fatal("close waited for an absent peer")
			}
		})
	}
}
