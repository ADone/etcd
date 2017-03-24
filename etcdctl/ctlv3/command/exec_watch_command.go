// Copyright 2017 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

// NewExecWatchCommand returns the cobra command for "execWatch".
func NewExecWatchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec-watch <key> <command> [args...]",
		Short: "Watch a key for changes and exec an executable",
		Run:   execWatchCommandFunc,
	}
	cmd.Flags().BoolVar(&watchPrefix, "prefix", false, "Watch on a prefix if prefix is set")
	cmd.Flags().Int64Var(&watchRev, "rev", 0, "Revision to start watching")
	cmd.Flags().BoolVar(&watchPrevKey, "prev-kv", false, "get the previous key-value pair before the event happens")
	return cmd
}

func execWatchCommandFunc(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		ExitWithError(ExitBadArgs, errors.New("execWatch takes one key name argument and a command name argument with optional arguments."))
	}
	c := mustClientFromCmd(cmd)

	ctx, cancel := context.WithCancel(context.TODO())
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill)
	go func() {
		<-sigc
		cancel()
	}()

	opts := []clientv3.OpOption{clientv3.WithRev(watchRev)}
	if watchPrefix {
		opts = append(opts, clientv3.WithPrefix())
	}
	if watchPrevKey {
		opts = append(opts, clientv3.WithPrevKV())
	}

	for response := range c.Watch(ctx, args[0], opts...) {
		if response.Canceled {
			ExitWithError(ExitError, response.Err())
		}

		if response.Created {
			continue
		}

		for _, event := range response.Events {
			cmd := exec.Command(args[1], args[1:]...)
			cmd.Env = environEvent(event, os.Environ())

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			go func() {
				err := cmd.Start()
				if err != nil {
					fmt.Fprintf(os.Stderr, err.Error())
					os.Exit(1)
				}
				cmd.Wait()
			}()
		}
	}

	if err := c.Close(); err != nil {
		ExitWithError(ExitBadConnection, err)
	}
}

func environEvent(event *clientv3.Event, env []string) []string {
	env = append(env, fmt.Sprintf("ETCD_KEY_ACTION=%s", event.Type))
	return environKV(event.Kv, env)
}

func environKV(kv *mvccpb.KeyValue, env []string) []string {
	env = append(env, fmt.Sprintf("ETCD_KEY_NAME=%s", kv.Key))
	env = append(env, fmt.Sprintf("ETCD_KEY_VALUE=%s", kv.Value))
	env = append(env, fmt.Sprintf("ETCD_KEY_VERSION=%d", kv.Version))
	env = append(env, fmt.Sprintf("ETCD_KEY_MOD_REVISION=%d", kv.ModRevision))
	env = append(env, fmt.Sprintf("ETCD_KEY_CREATE_REVISION=%d", kv.CreateRevision))
	return env
}
