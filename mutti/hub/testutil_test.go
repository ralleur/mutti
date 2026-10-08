// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"io"
)

func writeU32(w io.Writer, v uint32) error { return binary.Write(w, binary.BigEndian, v) }

func crc32IEEE(b []byte) uint32 { return crc32.ChecksumIEEE(b) }

func zlibBytes(b []byte) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}
