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

package task

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/cmd/taskrun"
	"github.com/tektoncd/cli/pkg/flags"
	"github.com/tektoncd/cli/pkg/helper/options"
	thelper "github.com/tektoncd/cli/pkg/helper/task"
	trlist "github.com/tektoncd/cli/pkg/helper/taskrun/list"
	validate "github.com/tektoncd/cli/pkg/helper/validate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func nameArg(args []string, p cli.Params) error {
	if len(args) == 1 {
		c, err := p.Clients()
		if err != nil {
			return err
		}
		name, ns := args[0], p.Namespace()
		if _, err = c.Tekton.TektonV1alpha1().Tasks(ns).Get(name, metav1.GetOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func logCommand(p cli.Params) *cobra.Command {
	opts := options.NewLogOptions(p)

	eg := `
  # interactive mode: shows logs of the selected taskrun
    tkn task logs -n namespace

  # interactive mode: shows logs of the selected taskrun of the given task
    tkn task logs task -n namespace

  # show logs of given task for last taskrun
    tkn task logs task -n namespace --last

  # show logs for given task and taskrun
    tkn task logs task taskrun -n namespace

   `
	c := &cobra.Command{
		Use:                   "logs",
		DisableFlagsInUseLine: true,
		Short:                 "Show task logs",
		Example:               eg,
		SilenceUsage:          true,
		Annotations: map[string]string{
			"commandType": "main",
		},
		Args: func(cmd *cobra.Command, args []string) error {
			if err := flags.InitParams(p, cmd); err != nil {
				return err
			}
			return nameArg(args, p)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Stream = &cli.Stream{
				Out: cmd.OutOrStdout(),
				Err: cmd.OutOrStderr(),
			}

			if err := validate.NamespaceExists(p); err != nil {
				return err
			}

			return run(opts, args)
		},
	}
	c.Flags().BoolVarP(&opts.Last, "last", "L", false, "show logs for last taskrun")
	c.Flags().BoolVarP(&opts.AllSteps, "all", "a", false, "show all logs including init steps injected by tekton")
	c.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "stream live logs")
	c.Flags().IntVarP(&opts.Limit, "limit", "", 5, "lists number of taskruns")

	_ = c.MarkZshCompPositionalArgumentCustom(1, "__tkn_get_task")
	return c
}

func run(opts *options.LogOptions, args []string) error {
	if err := initOpts(opts, args); err != nil {
		return err
	}

	if opts.TaskName == "" || opts.TaskrunName == "" {
		return nil
	}

	return taskrun.Run(opts)
}

func initOpts(opts *options.LogOptions, args []string) error {
	// ensure the client is properly initialized
	if _, err := opts.Params.Clients(); err != nil {
		return err
	}

	if err := opts.ValidateOpts(); err != nil {
		return err
	}

	switch len(args) {
	case 0: // no inputs
		return getAllInputs(opts)

	case 1: // task name provided
		opts.TaskName = args[0]
		return askRunName(opts)

	case 2: // both task and run provided
		opts.TaskName = args[0]
		opts.TaskrunName = args[1]

	default:
		return fmt.Errorf("too many arguments")
	}
	return nil
}

func getAllInputs(opts *options.LogOptions) error {
	ts, err := thelper.GetAllTaskNames(opts.Params)
	if err != nil {
		return err
	}

	if len(ts) == 0 {
		fmt.Fprintln(opts.Stream.Err, "No tasks found in namespace:", opts.Params.Namespace())
		return nil
	}

	if err := opts.Ask(options.ResourceNameTask, ts); err != nil {
		return nil
	}

	return askRunName(opts)
}

func askRunName(opts *options.LogOptions) error {
	if opts.Last {
		return initLastRunName(opts)
	}

	lOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("tekton.dev/task=%s", opts.TaskName),
	}

	trs, err := trlist.GetAllTaskRuns(opts.Params, lOpts, opts.Limit)
	if err != nil {
		return err
	}

	if len(trs) == 0 {
		fmt.Fprintln(opts.Stream.Err, "No taskruns found for task:", opts.TaskName)
		return nil
	}

	if len(trs) == 1 {
		opts.TaskrunName = strings.Fields(trs[0])[0]
		return nil
	}

	return opts.Ask(options.ResourceNameTaskRun, trs)
}

func initLastRunName(opts *options.LogOptions) error {
	cs, err := opts.Params.Clients()
	if err != nil {
		return err
	}
	lastrun, err := thelper.LastRun(cs.Tekton, opts.TaskName, opts.Params.Namespace())
	if err != nil {
		return err
	}
	opts.TaskrunName = lastrun.Name
	return nil
}
