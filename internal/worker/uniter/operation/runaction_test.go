// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	stdcontext "context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type RunActionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunActionSuite{})

func newOpFactory(c *gc.C, runnerFactory runner.Factory, callbacks operation.Callbacks) operation.Factory {
	actionResult := params.ActionResult{
		Action: &params.Action{Name: "backup"},
	}
	return newOpFactoryForAction(c, runnerFactory, callbacks, actionResult)
}

func newOpFactoryForAction(c *gc.C, runnerFactory runner.Factory, callbacks operation.Callbacks, action params.ActionResult) operation.Factory {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{action},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	return operation.NewFactory(operation.FactoryParams{
		ActionGetter:  client,
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
		Logger:        loggertesting.WrapCheckLog(c),
	})
}

func (s *RunActionSuite) TestPrepareErrorBadActionAndFailSucceeds(c *gc.C) {
	errBadAction := charmrunner.NewBadActionError("some-action-id", "splat")
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: errBadAction},
	}
	callbacks := &RunActionCallbacks{
		MockFailAction: &MockFailAction{err: errors.New("squelch")},
	}
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "squelch")
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
	c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, errBadAction.Error())
}

func (s *RunActionSuite) TestPrepareErrorBadActionAndFailErrors(c *gc.C) {
	errBadAction := charmrunner.NewBadActionError("some-action-id", "foof")
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: errBadAction},
	}
	callbacks := &RunActionCallbacks{
		MockFailAction: &MockFailAction{},
	}
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
	c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, errBadAction.Error())
}

func (s *RunActionSuite) TestPrepareErrorActionNotAvailable(c *gc.C) {
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: charmrunner.ErrActionNotAvailable},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
}

func (s *RunActionSuite) TestPrepareErrorOther(c *gc.C) {
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: errors.New("foop")},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `cannot create runner for action ".*": foop`)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
}

func (s *RunActionSuite) TestPrepareCtxCalled(c *gc.C) {
	ctx := &MockContext{actionData: &context.ActionData{Name: "some-action-name"}}
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunActionSuite) TestPrepareCtxError(c *gc.C) {
	ctx := &MockContext{actionData: &context.ActionData{Name: "some-action-name"}}
	ctx.SetErrors(errors.New("ctx prepare error"))
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.ErrorMatches, `ctx prepare error`)
	c.Assert(newState, gc.IsNil)
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunActionSuite) TestPrepareSuccessCleanState(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("should not call"))
	factory := newOpFactory(c, runnerFactory, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, jc.DeepEquals, &operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Pending,
		ActionId: &someActionId,
	})
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
}

func (s *RunActionSuite) TestPrepareSuccessDirtyState(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("should not call"))
	factory := newOpFactory(c, runnerFactory, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), overwriteState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, jc.DeepEquals, &operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Pending,
		ActionId: &someActionId,
		Started:  true,
		Hook:     &hook.Info{Kind: hooks.Install},
	})
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
}

func (s *RunActionSuite) TestExecuteSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "empty state",
		after: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Done,
			ActionId: &someActionId,
		},
	}, {
		description: "preserves appropriate fields",
		before:      overwriteState,
		after: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Done,
			ActionId: &someActionId,
			Hook:     &hook.Info{Kind: hooks.Install},
			Started:  true,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		runnerFactory := NewRunActionRunnerFactory(nil)
		callbacks := &RunActionCallbacks{}
		factory := newOpFactory(c, runnerFactory, callbacks)
		op, err := factory.NewAction(stdcontext.Background(), someActionId)
		c.Assert(err, jc.ErrorIsNil)
		midState, err := op.Prepare(stdcontext.Background(), test.before)
		c.Assert(midState, gc.NotNil)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(stdcontext.Background(), *midState)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, jc.DeepEquals, &test.after)
		c.Assert(callbacks.executingMessage, gc.Equals, "running action some-action-name")
		c.Assert(*runnerFactory.MockNewActionRunner.runner.MockRunAction.gotName, gc.Equals, "some-action-name")
		c.Assert(runnerFactory.MockNewActionRunner.gotCancel, gc.NotNil)
	}
}

func (s *RunActionSuite) TestExecuteCancel(c *gc.C) {
	actionChan := make(chan error)
	defer close(actionChan)
	runnerFactory := NewRunActionWaitRunnerFactory(actionChan)
	callbacks := &RunActionCallbacks{
		actionStatus: "running",
	}
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)
	midState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(midState, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	abortedErr := errors.Errorf("aborted")
	wait := make(chan struct{})
	go func() {
		newState, err := op.Execute(stdcontext.Background(), *midState)
		c.Assert(errors.Cause(err), gc.Equals, abortedErr)
		c.Assert(newState, gc.IsNil)
		c.Assert(runnerFactory.MockNewActionWaitRunner.runner.actionName, gc.Equals, "some-action-name")
		c.Assert(runnerFactory.MockNewActionWaitRunner.runner.actionChan, gc.Equals, (<-chan error)(actionChan))
		c.Assert(runnerFactory.MockNewActionWaitRunner.gotCancel, gc.NotNil)
		close(wait)
	}()

	op.RemoteStateChanged(remotestate.Snapshot{
		ActionChanged: map[string]int{
			someActionId: 1,
		},
	})

	callbacks.setActionStatus("aborting", nil)

	op.RemoteStateChanged(remotestate.Snapshot{
		ActionChanged: map[string]int{
			someActionId: 2,
		},
	})

	select {
	case <-runnerFactory.gotCancel:
	case <-time.After(testing.ShortWait):
		c.Fatalf("waiting for cancel")
	}

	select {
	case actionChan <- abortedErr:
	case <-time.After(testing.ShortWait):
		c.Fatalf("waiting for send")
	}

	select {
	case <-wait:
	case <-time.After(testing.ShortWait):
		c.Fatalf("waiting for finish")
	}
}

func (s *RunActionSuite) TestCommit(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "empty state",
		after: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		},
	}, {
		description: "preserves only appropriate fields, no hook",
		before: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			Started:  true,
			CharmURL: "ch:quantal/wordpress-2",
			ActionId: &randomActionId,
		},
		after: operation.State{
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Started: true,
		},
	}, {
		description: "preserves only appropriate fields, with hook",
		before: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			Started:  true,
			CharmURL: "ch:quantal/wordpress-2",
			ActionId: &randomActionId,
			Hook:     &hook.Info{Kind: hooks.Install},
		},
		after: operation.State{
			Kind:    operation.RunHook,
			Step:    operation.Pending,
			Hook:    &hook.Info{Kind: hooks.Install},
			Started: true,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		factory := newOpFactory(c, nil, nil)
		op, err := factory.NewAction(stdcontext.Background(), someActionId)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Commit(stdcontext.Background(), test.before)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, jc.DeepEquals, &test.after)
	}
}

func (s *RunActionSuite) TestNeedsGlobalMachineLock(c *gc.C) {
	factory := newOpFactory(c, nil, nil)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsTrue)
}

func (s *RunActionSuite) TestDoesNotNeedGlobalMachineLock(c *gc.C) {
	parallel := true
	actionResult := params.ActionResult{
		Action: &params.Action{Name: "backup", Parallel: &parallel},
	}
	factory := newOpFactoryForAction(c, nil, nil, actionResult)
	op, err := factory.NewAction(stdcontext.Background(), someActionId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsFalse)
}
