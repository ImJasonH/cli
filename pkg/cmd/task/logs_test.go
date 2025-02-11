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
	"testing"
	"time"

	"github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
	goexpect "github.com/Netflix/go-expect"
	"github.com/jonboulle/clockwork"
	"github.com/tektoncd/cli/pkg/cli"
	"github.com/tektoncd/cli/pkg/helper/options"
	"github.com/tektoncd/cli/pkg/helper/pods/fake"
	htest "github.com/tektoncd/cli/pkg/helper/test"
	"github.com/tektoncd/cli/pkg/test"
	cb "github.com/tektoncd/cli/pkg/test/builder"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipelinerun/resources"
	pipelinetest "github.com/tektoncd/pipeline/test"
	tb "github.com/tektoncd/pipeline/test/builder"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
)

func init() {
	// disable color output for all prompts to simplify testing
	core.DisableColor = true
}

func TestTaskLog(t *testing.T) {
	clock := clockwork.NewFakeClock()

	cs, _ := test.SeedTestData(t, pipelinetest.Data{
		Tasks: []*v1alpha1.Task{
			tb.Task("task", "ns"),
		},
		Namespaces: []*corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns",
				},
			},
		},
	})
	cs2, _ := test.SeedTestData(t, pipelinetest.Data{
		Tasks: []*v1alpha1.Task{
			tb.Task("task", "namespace"),
		},
		Namespaces: []*corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns",
				},
			},
		},
	})

	testParams := []struct {
		name      string
		command   []string
		input     pipelinetest.Clients
		wantError bool
		want      string
	}{
		{
			name:      "Invalid name space",
			command:   []string{"logs", "-n", "invalid"},
			input:     cs,
			wantError: true,
			want:      "namespaces \"invalid\" not found",
		},
		{
			name:      "Found no tasks",
			command:   []string{"logs", "-n", "ns"},
			input:     cs2,
			wantError: false,
			want:      "No tasks found in namespace: ns\n",
		},
		{
			name:      "Found no taskruns",
			command:   []string{"logs", "task", "-n", "ns"},
			input:     cs,
			wantError: false,
			want:      "No taskruns found for task: task\n",
		},
		{
			name:      "Specify notexist task name",
			command:   []string{"logs", "notexist", "-n", "ns"},
			input:     cs,
			wantError: true,
			want:      "tasks.tekton.dev \"notexist\" not found",
		},
		{
			name:      "Specify notexist taskrun name",
			command:   []string{"logs", "task", "notexist", "-n", "ns"},
			input:     cs,
			wantError: true,
			want:      "Unable to get Taskrun: taskruns.tekton.dev \"notexist\" not found",
		},
		{
			name:      "Specify negative number to limit",
			command:   []string{"logs", "task", "-n", "ns", "--limit", "-1"},
			input:     cs,
			wantError: true,
			want:      "limit was -1 but must be a positive number",
		},
	}

	for _, tp := range testParams {
		t.Run(tp.name, func(t *testing.T) {
			p := &test.Params{Tekton: tp.input.Pipeline, Clock: clock, Kube: tp.input.Kube}
			c := Command(p)

			out, err := test.ExecuteCommand(c, tp.command...)
			if tp.wantError {
				if err == nil {
					t.Errorf("error expected here")
				}
				test.AssertOutput(t, tp.want, err.Error())
			} else {
				if err != nil {
					t.Errorf("unexpected Error")
				}
				test.AssertOutput(t, tp.want, out)
			}
		})
	}
}

func TestTaskLog2(t *testing.T) {
	clock := clockwork.NewFakeClock()

	cs, _ := test.SeedTestData(t, pipelinetest.Data{
		Tasks: []*v1alpha1.Task{
			tb.Task("task", "ns", cb.TaskCreationTime(clock.Now())),
		},
		TaskRuns: []*v1alpha1.TaskRun{
			tb.TaskRun("taskrun1", "ns",
				tb.TaskRunLabel("tekton.dev/task", "task"),
				tb.TaskRunSpec(tb.TaskRunTaskRef("task")),
				tb.TaskRunStatus(
					tb.PodName("pod"),
					tb.TaskRunStartTime(clock.Now().Add(-5*time.Minute)),
					tb.StatusCondition(apis.Condition{
						Status: corev1.ConditionTrue,
						Reason: resources.ReasonSucceeded,
					}),
					tb.StepState(
						cb.StepName("step1"),
						tb.StateTerminated(0),
					),
				),
			),
			tb.TaskRun("taskrun2", "ns",
				tb.TaskRunLabel("tekton.dev/task", "task"),
				tb.TaskRunSpec(tb.TaskRunTaskRef("task")),
				tb.TaskRunStatus(
					tb.PodName("pod"),
					tb.TaskRunStartTime(clock.Now().Add(-3*time.Minute)),
					tb.StatusCondition(apis.Condition{
						Status: corev1.ConditionTrue,
						Reason: resources.ReasonSucceeded,
					}),
					tb.StepState(
						cb.StepName("step2"),
						tb.StateTerminated(0),
					),
				),
			),
		},
		Namespaces: []*corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns",
				},
			},
		},
		Pods: []*corev1.Pod{
			tb.Pod("pod", "ns",
				tb.PodSpec(
					tb.PodContainer("step1", "step1:latest"),
					tb.PodContainer("step2", "step2:latest"),
				),
				cb.PodStatus(
					cb.PodPhase(corev1.PodSucceeded),
				),
			),
		},
	})

	cs2, _ := test.SeedTestData(t, pipelinetest.Data{
		Tasks: []*v1alpha1.Task{
			tb.Task("task", "ns", cb.TaskCreationTime(clock.Now())),
		},
		TaskRuns: []*v1alpha1.TaskRun{
			tb.TaskRun("taskrun1", "ns",
				tb.TaskRunLabel("tekton.dev/task", "task"),
				tb.TaskRunSpec(tb.TaskRunTaskRef("task")),
				tb.TaskRunStatus(
					tb.PodName("pod"),
					tb.TaskRunStartTime(clock.Now().Add(-5*time.Minute)),
					tb.StatusCondition(apis.Condition{
						Status: corev1.ConditionTrue,
						Reason: resources.ReasonSucceeded,
					}),
					tb.StepState(
						cb.StepName("step1"),
						tb.StateTerminated(0),
					),
				),
			),
		},
		Namespaces: []*corev1.Namespace{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns",
				},
			},
		},
		Pods: []*corev1.Pod{
			tb.Pod("pod", "ns",
				tb.PodSpec(
					tb.PodContainer("step1", "step1:latest"),
					tb.PodContainer("step2", "step2:latest"),
				),
				cb.PodStatus(
					cb.PodPhase(corev1.PodSucceeded),
				),
			),
		},
	})

	logs := fake.Logs(
		fake.Task("pod",
			fake.Step("step1", "step1 log"),
			fake.Step("step2", "step2 log"),
		),
	)

	testParams := []struct {
		name      string
		limit     int
		last      bool
		namespace string
		input     pipelinetest.Clients
		prompt    htest.PromptTest
	}{
		{
			name:      "Get all input",
			limit:     5,
			last:      false,
			namespace: "ns",
			input:     cs,
			prompt: htest.PromptTest{
				CmdArgs: []string{},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectString("Select task:"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("task"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyEnter)); err != nil {
						return err
					}

					if _, err := c.ExpectString("Select taskrun:"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun2 started 3 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun1 started 5 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowUp)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun2 started 3 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyEnter)); err != nil {
						return err
					}

					return nil
				},
			},
		},
		{
			name:      "Specify task name and choice taskrun name from interactive menu",
			limit:     5,
			last:      false,
			namespace: "ns",
			input:     cs,
			prompt: htest.PromptTest{
				CmdArgs: []string{"task"},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectString("Select taskrun:"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun2 started 3 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun1 started 5 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyEnter)); err != nil {
						return err
					}

					return nil
				},
			},
		},
		{
			name:      "Specify task name and limit as 2",
			limit:     2,
			last:      false,
			namespace: "ns",
			input:     cs,
			prompt: htest.PromptTest{
				CmdArgs: []string{"task"},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectString("Select taskrun:"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun2 started 3 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("taskrun1 started 5 minutes ago"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyEnter)); err != nil {
						return err
					}

					return nil
				},
			},
		},
		{
			name:      "Specify task name and limit as 1",
			limit:     1,
			last:      false,
			namespace: "ns",
			input:     cs,
			prompt: htest.PromptTest{
				CmdArgs: []string{"task"},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectString("step1 log\r\n"); err != nil {
						return err
					}

					if _, err := c.ExpectString("step2 log\r\n"); err != nil {
						return err
					}

					if _, err := c.ExpectEOF(); err != nil {
						return err
					}

					return nil
				},
			},
		},
		{
			name:      "Specify last flag as true",
			limit:     5,
			last:      true,
			namespace: "ns",
			input:     cs,
			prompt: htest.PromptTest{
				CmdArgs: []string{},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectString("Select task:"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyArrowDown)); err != nil {
						return err
					}

					if _, err := c.ExpectString("task"); err != nil {
						return err
					}

					if _, err := c.SendLine(string(terminal.KeyEnter)); err != nil {
						return err
					}

					if _, err := c.ExpectEOF(); err != nil {
						return err
					}

					return nil
				},
			},
		},
		{
			name:      "Specify last flag as true and task name",
			limit:     5,
			last:      true,
			namespace: "ns",
			input:     cs,
			prompt: htest.PromptTest{
				CmdArgs: []string{"task"},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectEOF(); err != nil {
						return err
					}

					return nil
				},
			},
		},
		{
			name:      "Specify task name when taskrun is single",
			limit:     5,
			last:      false,
			namespace: "ns",
			input:     cs2,
			prompt: htest.PromptTest{
				CmdArgs: []string{"task"},
				Procedure: func(c *goexpect.Console) error {
					if _, err := c.ExpectEOF(); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}

	for _, tp := range testParams {
		t.Run(tp.name, func(t *testing.T) {
			p := test.Params{
				Kube:   tp.input.Kube,
				Tekton: tp.input.Pipeline,
			}
			p.SetNamespace(tp.namespace)

			opts := &options.LogOptions{
				Limit:    tp.limit,
				Last:     tp.last,
				Params:   &p,
				Streamer: fake.Streamer(logs),
			}

			tp.prompt.RunTest(t, tp.prompt.Procedure, func(stdio terminal.Stdio) error {
				opts.AskOpts = htest.WithStdio(stdio)
				opts.Stream = &cli.Stream{Out: stdio.Out, Err: stdio.Err}

				err := run(opts, tp.prompt.CmdArgs)
				if err != nil {
					return err
				}
				return err
			})
		})
	}
}
