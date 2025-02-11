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
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/ghodss/yaml"

	"github.com/spf13/cobra"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/cmd/taskrun"
	"github.com/tektoncd/cli/pkg/flags"
	"github.com/tektoncd/cli/pkg/helper/labels"
	"github.com/tektoncd/cli/pkg/helper/options"
	"github.com/tektoncd/cli/pkg/helper/params"
	"github.com/tektoncd/cli/pkg/helper/task"
	validate "github.com/tektoncd/cli/pkg/helper/validate"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	errNoTask      = errors.New("missing task name")
	errInvalidTask = "task name %s does not exist in namespace %s"
)

const invalidResource = "invalid input format for resource parameter: "

type startOptions struct {
	cliparams          cli.Params
	stream             *cli.Stream
	Params             []string
	InputResources     []string
	OutputResources    []string
	ServiceAccountName string
	Last               bool
	Labels             []string
	ShowLog            bool
	Filename           string
	TimeOut            int64
}

// NameArg validates that the first argument is a valid task name
func NameArg(args []string, p cli.Params) error {
	if len(args) == 0 {
		return errNoTask
	}

	if err := validate.NamespaceExists(p); err != nil {
		return err
	}

	c, err := p.Clients()
	if err != nil {
		return err
	}

	name, ns := args[0], p.Namespace()
	t, err := c.Tekton.TektonV1alpha1().Tasks(ns).Get(name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf(errInvalidTask, name, ns)
	}

	if t.Spec.Inputs != nil {
		params.FilterParamsByType(t.Spec.Inputs.Params)
	}

	return nil
}

func startCommand(p cli.Params) *cobra.Command {
	opt := startOptions{
		cliparams: p,
	}

	c := &cobra.Command{
		Use:     "start task [RESOURCES...] [PARAMS...] [SERVICEACCOUNT]",
		Aliases: []string{"trigger"},
		Short:   "Start tasks",
		Annotations: map[string]string{
			"commandType": "main",
		},
		Example: `
# start task foo by creating a taskrun named "foo-run-xyz123" from the namespace "bar"
tkn task start foo -s ServiceAccountName -n bar

The task can either be specified by reference in a cluster using the positional argument
or in a file using the --filename argument.

For params value, if you want to provide multiple values, provide them comma separated
like cat,foo,bar
`,
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := flags.InitParams(p, cmd); err != nil {
				return err
			}
			if len(args) != 0 {
				return NameArg(args, p)
			}
			if opt.Filename == "" {
				return errors.New("Either a task name or a --filename parameter must be supplied")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opt.stream = &cli.Stream{
				Out: cmd.OutOrStdout(),
				Err: cmd.OutOrStderr(),
			}

			return startTask(opt, args)
		},
	}

	c.Flags().StringSliceVarP(&opt.InputResources, "inputresource", "i", []string{}, "pass the input resource name and ref as name=ref")
	c.Flags().StringSliceVarP(&opt.OutputResources, "outputresource", "o", []string{}, "pass the output resource name and ref as name=ref")
	c.Flags().StringArrayVarP(&opt.Params, "param", "p", []string{}, "pass the param as key=value or key=value1,value2")
	c.Flags().StringVarP(&opt.ServiceAccountName, "serviceaccount", "s", "", "pass the serviceaccount name")
	flags.AddShellCompletion(c.Flags().Lookup("serviceaccount"), "__kubectl_get_serviceaccount")
	c.Flags().BoolVarP(&opt.Last, "last", "L", false, "re-run the task using last taskrun values")
	c.Flags().StringSliceVarP(&opt.Labels, "labels", "l", []string{}, "pass labels as label=value.")
	c.Flags().BoolVarP(&opt.ShowLog, "showlog", "", true, "show logs right after starting the task")
	c.Flags().StringVarP(&opt.Filename, "filename", "f", "", "filename containing a task definition")
	c.Flags().Int64VarP(&opt.TimeOut, "timeout", "t", 3600, "timeout for taskrun in seconds")

	_ = c.MarkZshCompPositionalArgumentCustom(1, "__tkn_get_task")

	return c
}

func parseTask(p string) (*v1alpha1.Task, error) {
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	task := v1alpha1.Task{}
	if err := yaml.Unmarshal(b, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func startTask(opt startOptions, args []string) error {
	tr := &v1alpha1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: opt.cliparams.Namespace(),
		},
	}

	var tname string
	timeoutSeconds := time.Duration(opt.TimeOut) * time.Second

	if len(args) > 0 {
		tname = args[0]
		tr.Spec = v1alpha1.TaskRunSpec{
			TaskRef: &v1alpha1.TaskRef{Name: tname},
			Timeout: &metav1.Duration{Duration: timeoutSeconds},
		}
	} else {
		task, err := parseTask(opt.Filename)
		if err != nil {
			return err
		}
		tname = task.ObjectMeta.Name
		tr.Spec = v1alpha1.TaskRunSpec{
			TaskSpec: &task.Spec,
		}
	}
	tr.ObjectMeta.GenerateName = tname + "-run-"

	cs, err := opt.cliparams.Clients()
	if err != nil {
		return err
	}

	if opt.Last {
		trLast, err := task.LastRun(cs.Tekton, tname, opt.cliparams.Namespace())
		if err != nil {
			return err
		}
		tr.Spec.Inputs = trLast.Spec.Inputs
		tr.Spec.Outputs = trLast.Spec.Outputs
		tr.Spec.ServiceAccountName = trLast.Spec.ServiceAccountName
	}

	inputRes, err := mergeRes(tr.Spec.Inputs.Resources, opt.InputResources)
	if err != nil {
		return err
	}
	tr.Spec.Inputs.Resources = inputRes

	outRes, err := mergeRes(tr.Spec.Outputs.Resources, opt.OutputResources)
	if err != nil {
		return err
	}
	tr.Spec.Outputs.Resources = outRes

	labels, err := labels.MergeLabels(tr.ObjectMeta.Labels, opt.Labels)
	if err != nil {
		return err
	}
	tr.ObjectMeta.Labels = labels

	param, err := params.MergeParam(tr.Spec.Inputs.Params, opt.Params)
	if err != nil {
		return err
	}
	tr.Spec.Inputs.Params = param

	if len(opt.ServiceAccountName) > 0 {
		tr.Spec.ServiceAccountName = opt.ServiceAccountName
	}

	trCreated, err := cs.Tekton.TektonV1alpha1().TaskRuns(opt.cliparams.Namespace()).Create(tr)
	if err != nil {
		return err
	}

	fmt.Fprintf(opt.stream.Out, "Taskrun started: %s\n", trCreated.Name)
	if !opt.ShowLog {
		fmt.Fprintf(opt.stream.Out, "\nIn order to track the taskrun progress run:\ntkn taskrun logs %s -f -n %s\n", trCreated.Name, trCreated.Namespace)
		return nil
	}

	fmt.Fprintf(opt.stream.Out, "Waiting for logs to be available...\n")
	runLogOpts := &options.LogOptions{
		TaskrunName: trCreated.Name,
		Stream:      opt.stream,
		Follow:      true,
		Params:      opt.cliparams,
		AllSteps:    false,
	}
	return taskrun.Run(runLogOpts)
}

func mergeRes(r []v1alpha1.TaskResourceBinding, optRes []string) ([]v1alpha1.TaskResourceBinding, error) {
	res, err := parseRes(optRes)
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return r, nil
	}

	for i := range r {
		if v, ok := res[r[i].Name]; ok {
			r[i] = v
			delete(res, v.Name)
		}
	}
	for _, v := range res {
		r = append(r, v)
	}
	return r, nil
}

func parseRes(res []string) (map[string]v1alpha1.TaskResourceBinding, error) {
	resources := map[string]v1alpha1.TaskResourceBinding{}
	for _, v := range res {
		r := strings.SplitN(v, "=", 2)
		if len(r) != 2 {
			return nil, errors.New(invalidResource + v)
		}
		resources[r[0]] = v1alpha1.TaskResourceBinding{
			PipelineResourceBinding: v1alpha1.PipelineResourceBinding{
				Name: r[0],
				ResourceRef: &v1alpha1.PipelineResourceRef{
					Name: r[1],
				},
			},
		}
	}
	return resources, nil
}
