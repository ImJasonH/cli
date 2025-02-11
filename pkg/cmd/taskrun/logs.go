// Copyright © 2019 The Tekton Authors.
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

package taskrun

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/helper/options"
	"github.com/tektoncd/cli/pkg/helper/pods"
	trlist "github.com/tektoncd/cli/pkg/helper/taskrun/list"
	validate "github.com/tektoncd/cli/pkg/helper/validate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	msgTRNotFoundErr = "Unable to get Taskrun"
)

func logCommand(p cli.Params) *cobra.Command {
	opts := &options.LogOptions{Params: p}
	eg := `
# show the logs of TaskRun named "foo" from the namespace "bar"
tkn taskrun logs foo -n bar

# show the live logs of TaskRun named "foo" from the namespace "bar"
tkn taskrun logs -f foo -n bar
`
	c := &cobra.Command{
		Use:          "logs",
		Short:        "Show taskruns logs",
		Example:      eg,
		SilenceUsage: true,
		Annotations: map[string]string{
			"commandType": "main",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				opts.TaskrunName = args[0]
			}

			opts.Stream = &cli.Stream{
				Out: cmd.OutOrStdout(),
				Err: cmd.OutOrStderr(),
			}

			if err := validate.NamespaceExists(p); err != nil {
				return err
			}

			return Run(opts)
		},
	}

	c.Flags().BoolVarP(&opts.AllSteps, "all", "a", false, "show all logs including init steps injected by tekton")
	c.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "stream live logs")
	c.Flags().IntVarP(&opts.Limit, "limit", "", 5, "lists number of taskruns")

	_ = c.MarkZshCompPositionalArgumentCustom(1, "__tkn_get_taskrun")
	return c
}

func Run(opts *options.LogOptions) error {
	if opts.TaskrunName == "" {
		if err := askRunName(opts); err != nil {
			return err
		}
	}

	streamer := pods.NewStream
	if opts.Streamer != nil {
		streamer = opts.Streamer
	}

	cs, err := opts.Params.Clients()
	if err != nil {
		return err
	}

	lr := &LogReader{
		Run:      opts.TaskrunName,
		Ns:       opts.Params.Namespace(),
		Clients:  cs,
		Streamer: streamer,
		Stream:   opts.Stream,
		Follow:   opts.Follow,
		AllSteps: opts.AllSteps,
	}

	logC, errC, err := lr.Read()
	if err != nil {
		return err
	}

	NewLogWriter().Write(opts.Stream, logC, errC)
	return nil
}

func askRunName(opts *options.LogOptions) error {
	lOpts := metav1.ListOptions{}

	trs, err := trlist.GetAllTaskRuns(opts.Params, lOpts, opts.Limit)
	if err != nil {
		return err
	}

	if len(trs) == 0 {
		return fmt.Errorf("No taskruns found")
	}

	if len(trs) == 1 {
		opts.TaskrunName = strings.Fields(trs[0])[0]
		return nil
	}

	return opts.Ask(options.ResourceNameTaskRun, trs)
}
