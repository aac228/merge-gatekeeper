package status

import (
	"context"
	"errors"
	"fmt"

	"github.com/aac228/merge-gatekeeper/internal/github"
	"github.com/aac228/merge-gatekeeper/internal/multierror"
	"github.com/aac228/merge-gatekeeper/internal/validators"
)

const (
	successState = "success"
	errorState   = "error"
	failureState = "failure"
	pendingState = "pending"
)

// NOTE: https://docs.github.com/en/rest/reference/checks
const (
	checkRunCompletedStatus  = "completed"
	checkRunQueuedStatus     = "queued"
	checkRunInProgressStatus = "in_progress"
)
const (
	checkRunNeutralConclusion  = "neutral"
	checkRunSuccessConclusion  = "success"
	checkRunSkipConclusion     = "skipped"
	checkRunFailedConclusion   = "failure"
	checkRunTimedOutConclusion = "timed_out"
)

const (
	maxStatusesPerPage  = 100
	maxCheckRunsPerPage = 100
)

var (
	ErrInvalidCombinedStatusResponse = errors.New("github combined status response is invalid")
	ErrInvalidCheckRunResponse       = errors.New("github checkRun response is invalid")
)

type ghaStatus struct {
	Job      string
	Workflow string
	State    string
}

func (gs *ghaStatus) String() string {
	return fmt.Sprintf("%s / %s", gs.Workflow, gs.Job)
}

type statusValidator struct {
	repo        string
	owner       string
	ref         string
	selfJobName string
	ignoredJobs []string
	client      github.Client
}

func CreateValidator(c github.Client, opts ...Option) (validators.Validator, error) {
	sv := &statusValidator{
		client: c,
	}
	for _, opt := range opts {
		opt(sv)
	}
	if err := sv.validateFields(); err != nil {
		return nil, err
	}
	return sv, nil
}

func (sv *statusValidator) Name() string {
	return sv.selfJobName
}

func (sv *statusValidator) validateFields() error {
	errs := make(multierror.Errors, 0, 6)

	if len(sv.repo) == 0 {
		errs = append(errs, errors.New("repository name is empty"))
	}
	if len(sv.owner) == 0 {
		errs = append(errs, errors.New("repository owner is empty"))
	}
	if len(sv.ref) == 0 {
		errs = append(errs, errors.New("reference of repository is empty"))
	}
	if len(sv.selfJobName) == 0 {
		errs = append(errs, errors.New("self job name is empty"))
	}
	if sv.client == nil {
		errs = append(errs, errors.New("github client is empty"))
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (sv *statusValidator) Validate(ctx context.Context) (validators.Status, error) {
	ghaStatuses, err := sv.listGhaStatuses(ctx)
	if err != nil {
		return nil, err
	}

	st := &status{
		totalJobs:    make([]string, 0, len(ghaStatuses)),
		completeJobs: make([]string, 0, len(ghaStatuses)),
		errJobs:      make([]string, 0, len(ghaStatuses)/2),
		ignoredJobs:  make([]string, 0, len(ghaStatuses)),
		succeeded:    true,
	}

	st.ignoredJobs = append(st.ignoredJobs, sv.ignoredJobs...)

	var successCnt int
	for _, ghaStatus := range ghaStatuses {
		var toIgnore bool
		for _, ignored := range sv.ignoredJobs {
			if ghaStatus.Job == ignored {
				toIgnore = true
				break
			}
		}

		// Ignored jobs and this job itself should be considered as success regardless of their statuses.
		if toIgnore || ghaStatus.Job == sv.selfJobName {
			successCnt++
			continue
		}

		st.totalJobs = append(st.totalJobs, ghaStatus.String())

		switch ghaStatus.State {
		case successState:
			st.completeJobs = append(st.completeJobs, ghaStatus.String())
			successCnt++
		case errorState, failureState:
			st.errJobs = append(st.errJobs, ghaStatus.String())
		}
	}
	if len(st.errJobs) != 0 {
		return nil, errors.New(st.Detail())
	}

	if len(ghaStatuses) != successCnt {
		st.succeeded = false
		return st, nil
	}

	return st, nil
}

func (sv *statusValidator) listCheckRunsForRef(ctx context.Context) ([]*github.CheckRun, error) {
	var runResults []*github.CheckRun
	page := 1
	for {
		cr, _, err := sv.client.ListCheckRunsForRef(ctx, sv.owner, sv.repo, sv.ref, &github.ListCheckRunsOptions{ListOptions: github.ListOptions{
			Page:    page,
			PerPage: maxCheckRunsPerPage,
		}})
		if err != nil {
			return nil, err
		}
		runResults = append(runResults, cr.CheckRuns...)
		if cr.GetTotal() <= len(runResults) {
			break
		}
		page++
	}
	return runResults, nil
}

func (sv *statusValidator) listGhaStatuses(ctx context.Context) ([]*ghaStatus, error) {
	currentJobs := make(map[string]struct{})

	// Get all the checks related to this reference
	runResults, err := sv.listCheckRunsForRef(ctx)
	if err != nil {
		return nil, err
	}

	ghaStatuses := make([]*ghaStatus, 0, len(runResults))

	// Get all the workflows related to this reference, this allows us to map the check suite ID to the workflow name
	workflowRuns, _, err := sv.client.ListWorkflowRuns(ctx, sv.owner, sv.repo, &github.ListWorkflowRunsOptions{
		HeadSHA: sv.ref,
	})

	if err != nil {
		return nil, err
	}

	// Map check suite ID to workflow name
	suiteToWorkflow := make(map[int64]string)
	fmt.Println("Found workflows:")
	for _, wf := range workflowRuns.WorkflowRuns {
		fmt.Println("-", wf.GetName())
		suiteToWorkflow[wf.GetCheckSuiteID()] = wf.GetName()
	}

	for _, run := range runResults {
		checkKey, wfName, err := CreateCheckKey(run, suiteToWorkflow)
		if err != nil {
			return nil, err
		}

		if run.Name == nil || run.Status == nil {
			return nil, fmt.Errorf("%w name: %v, status: %v", ErrInvalidCheckRunResponse, run.Name, run.Status)
		}
		if _, ok := currentJobs[checkKey]; ok {
			continue
		}
		currentJobs[checkKey] = struct{}{}

		ghaStatus := &ghaStatus{Job: *run.Name, Workflow: wfName}

		if *run.Status != checkRunCompletedStatus {
			ghaStatus.State = pendingState
			ghaStatuses = append(ghaStatuses, ghaStatus)
			continue
		}

		switch *run.Conclusion {
		case checkRunNeutralConclusion, checkRunSuccessConclusion:
			ghaStatus.State = successState
		case checkRunSkipConclusion:
			continue
		default:
			ghaStatus.State = errorState
		}
		ghaStatuses = append(ghaStatuses, ghaStatus)
	}

	return ghaStatuses, nil
}

func CreateCheckKey(run *github.CheckRun, suiteToWorkflow map[int64]string) (string, string, error) {
	checkSuiteID := run.GetCheckSuite().GetID()
	wfName, ok := suiteToWorkflow[checkSuiteID]

	fmt.Println("Found associated workflows:")
	for _, v := range suiteToWorkflow {
		fmt.Println("-", v)
	}

	if !ok {
		return "", "", fmt.Errorf("workflow name not found for check suite ID: %v of run %v", checkSuiteID, *run.Name)
	}

	return fmt.Sprintf("%v / %v", wfName, *run.Name), wfName, nil
}
