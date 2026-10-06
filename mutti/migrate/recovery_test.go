// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"testing"
	"time"
)

func TestRecoveryBudget(t *testing.T) {
	var b recoveryBudget
	now := time.Now()
	if !b.allow(now) || b.allow(now) || b.allow(now.Add(time.Second)) {
		t.Fatal("first backoff")
	}
	if !b.allow(now.Add(2*time.Second)) || b.allow(now.Add(3*time.Second)) {
		t.Fatal("second backoff")
	}
	if !b.allow(now.Add(12*time.Second)) || b.allow(now.Add(time.Minute)) {
		t.Fatal("crash loop must stop")
	}
	if !b.allow(now.Add(5 * time.Minute)) {
		t.Fatal("recovery window did not reset")
	}
}
