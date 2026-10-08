// SPDX-License-Identifier: MPL-2.0

// Command mutti-testdevice pairs like the kurtz client and exposes the same
// local capability gateway, for automated end-to-end tests of a synthetic
// Mutti instance. It is not part of any package.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	connect "github.com/ralleur/mutti/connect"
)

func main() {
	invite := flag.String("invite", "", "invitation URL from the owner")
	name := flag.String("name", "Testgerät", "device name shown to the owner")
	credentialsPath := flag.String("credentials", "", "private credentials file (created on pairing, reused afterwards)")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var credentials connect.Credentials
	if b, err := os.ReadFile(*credentialsPath); err == nil {
		if err = json.Unmarshal(b, &credentials); err != nil {
			fail(err)
		}
	} else {
		var err error
		if credentials, err = connect.NewCredentials(*invite); err != nil {
			fail(err)
		}
	}
	client, err := connect.NewClient(credentials)
	if err != nil {
		fail(err)
	}
	defer client.Close()
	if *invite != "" {
		fmt.Println(`{"state":"pairing"}`)
		if err = client.Pair(ctx, *name, nil); err != nil {
			fail(err)
		}
		b, _ := json.Marshal(client.Credentials())
		if err = os.WriteFile(*credentialsPath, b, 0600); err != nil {
			fail(err)
		}
	}
	gateway, err := client.Gateway()
	if err != nil {
		fail(err)
	}
	out, _ := json.Marshal(map[string]string{"state": "ready", "gateway": gateway, "pin": credentials.Identity.Pin()})
	fmt.Println(string(out))
	<-ctx.Done()
}

func fail(err error) {
	out, _ := json.Marshal(map[string]string{"state": "error", "message": err.Error()})
	fmt.Println(string(out))
	os.Exit(1)
}
