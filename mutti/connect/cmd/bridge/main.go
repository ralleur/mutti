// SPDX-License-Identifier: MPL-2.0
package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"encoding/json"
	connect "github.com/ralleur/mutti/connect"
	"sync"
	"unsafe"
)

type handle struct {
	mu          sync.Mutex
	client      *connect.Client
	cancel      context.CancelFunc
	state       string
	message     string
	credentials connect.Credentials
	url         string
}

var handles = struct {
	sync.Mutex
	next  uint64
	items map[uint64]*handle
}{items: map[uint64]*handle{}}

func result(v any) *C.char { b, _ := json.Marshal(v); return C.CString(string(b)) }

//export MCStart
func MCStart(input *C.char, pair C.int, name *C.char) *C.char {
	var credentials connect.Credentials
	var e error
	if pair != 0 {
		credentials, e = connect.NewCredentials(C.GoString(input))
	} else {
		e = json.Unmarshal([]byte(C.GoString(input)), &credentials)
	}
	if e != nil {
		return result(map[string]string{"error": e.Error()})
	}
	client, e := connect.NewClient(credentials)
	if e != nil {
		return result(map[string]string{"error": e.Error()})
	}
	address, e := client.Gateway()
	if e != nil {
		client.Close()
		return result(map[string]string{"error": e.Error()})
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &handle{client: client, cancel: cancel, state: "ready", url: address, credentials: client.Credentials()}
	handles.Lock()
	handles.next++
	id := handles.next
	handles.items[id] = h
	handles.Unlock()
	if pair != 0 {
		h.state = "connecting"
		deviceName := C.GoString(name)
		go func() {
			e := client.Pair(ctx, deviceName, func(message string) { h.mu.Lock(); h.state = "approval"; h.message = message; h.mu.Unlock() })
			h.mu.Lock()
			defer h.mu.Unlock()
			if e != nil {
				h.state = "error"
				h.message = e.Error()
			} else {
				h.state = "ready"
				h.message = ""
			}
		}()
	}
	return result(map[string]any{"handle": id, "url": address, "pin": credentials.Invitation.Pin, "devicePin": credentials.Identity.Pin()})
}

//export MCStatus
func MCStatus(id C.ulonglong) *C.char {
	handles.Lock()
	h := handles.items[uint64(id)]
	handles.Unlock()
	if h == nil {
		return result(map[string]string{"state": "error", "message": "Verbindung wurde beendet."})
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return result(map[string]any{"state": h.state, "message": h.message, "credentials": h.credentials})
}

//export MCStop
func MCStop(id C.ulonglong) {
	handles.Lock()
	h := handles.items[uint64(id)]
	delete(handles.items, uint64(id))
	handles.Unlock()
	if h != nil {
		h.cancel()
		go h.client.Close()
	}
}

//export MCFree
func MCFree(p *C.char) { C.free(unsafe.Pointer(p)) }
func main()            {}
