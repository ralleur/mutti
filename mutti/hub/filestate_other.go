// SPDX-License-Identifier: GPL-2.0-or-later
//go:build !darwin

package hub

import "syscall"

func ctime(st *syscall.Stat_t) (int64, int64) { return int64(st.Ctim.Sec), int64(st.Ctim.Nsec) }
