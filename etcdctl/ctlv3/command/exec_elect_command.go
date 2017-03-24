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

	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

// NewExecElectCommand returns the cobra command for "exec-elect".
func NewExecElectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec-elect <election-name> <command> [args...]",
		Short: "Observes leader election",
		Run:   execElectCommandFunc,
	}
	return cmd
}

func execElectCommandFunc(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		ExitWithError(ExitBadArgs, errors.New("exec-elect takes one election name argument and a command name argument with optional arguments."))
	}
	client := mustClientFromCmd(cmd)

	ctx, cancel := context.WithCancel(context.TODO())
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill)
	go func() {
		defer cancel()
		<-sigc
	}()

	session, err := concurrency.NewSession(client)
	if err != nil {
		ExitWithError(ExitBadConnection, err)
	}

	election := concurrency.NewElection(session, args[0])
	done := make(chan struct{})

	go func() {
		defer close(done)
		for response := range election.Observe(ctx) {
			if len(response.Kvs) != 0 {
				cmd := exec.Command(args[1], args[1:]...)
				cmd.Env = environKV(response.Kvs[0], os.Environ())

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
	}()

	<-done

	select {
	case <-ctx.Done():
	default:
		ExitWithError(ExitBadConnection, errors.New("elect: observer lost"))
	}

	if err := client.Close(); err != nil {
		ExitWithError(ExitBadConnection, err)
	}
}
