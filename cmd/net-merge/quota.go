package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net-merge/internal/i18n"
	"os"
	"sort"
	"time"

	"net-merge/internal/engine"
	"net-merge/internal/platform"
	"net-merge/internal/state"
)

func cmdQuota(args []string) {
	fs := newFlags("quota", flag.ExitOnError)
	dir := fs.String("state-dir", state.DefaultDir(), i18n.T("state directory"))
	fs.Parse(args)
	rest := fs.Args()
	records, err := state.LoadQuotas(*dir)
	if err != nil {
		die(err)
	}
	if len(rest) == 0 {
		ids := make([]string, 0, len(records))
		for id := range records {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if len(ids) == 0 {
			fmt.Println(i18n.T("No data allowances set."))
		}
		for _, id := range ids {
			r := records[id]
			limit := i18n.T("unlimited")
			if r.LimitBytes > 0 {
				limit = fmt.Sprintf(i18n.T("%.2f GB"), float64(r.LimitBytes)/1e9)
			}
			fmt.Printf(i18n.T("%s [%s]: used %.2f GB / %s\n"), r.Name, id, float64(r.UsedBytes)/1e9, limit)
		}
		return
	}
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, i18n.T("usage: net-merge quota <adapter> <GB|unlimited|reset>"))
		os.Exit(2)
	}
	r := state.QuotaRequest{Adapter: rest[0], Action: "set"}
	if rest[1] == "reset" {
		r.Action = "reset"
	} else {
		r.Bytes, err = engine.ParseQuota(rest[1])
		if err != nil {
			die(err)
		}
	}
	_, active := state.ReadAny(*dir)
	if platform.InstanceRunning() || active {
		// A unique receipt avoids treating an old zero counter as proof that reset ran.
		r.RequestID, err = state.NewQuotaRequestID()
		if err != nil {
			die(err)
		}
		receipt := state.QuotaReceiptPath(*dir)
		_ = os.Remove(receipt)
		if err := state.SendQuota(*dir, r); err != nil {
			die(err)
		}
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			b, e := os.ReadFile(receipt)
			if e == nil {
				var reply state.QuotaReceipt
				if json.Unmarshal(b, &reply) != nil || reply.RequestID != r.RequestID {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				if reply.Error != "" {
					die(fmt.Errorf(i18n.T("data allowance: %s"), reply.Error))
				}
				fmt.Println(i18n.T("Data allowance updated."))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		die(fmt.Errorf("%s", i18n.T("sent the allowance, but the running instance did not apply it")))
	}
	// With no running process, discover stable IDs without starting the network engine.
	var c common
	c.weights = weightFlags{}
	ns, e := c.nics()
	if e != nil {
		die(e)
	}
	sched := engine.NewScheduler(ns)
	sched.LoadQuotas(records)
	if err := state.ApplyQuota(*dir, sched, r); err != nil {
		die(err)
	}
	fmt.Println(i18n.T("Data allowance saved. It takes effect the next time net-merge starts."))
}
