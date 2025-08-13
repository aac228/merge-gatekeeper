package status

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/aac228/merge-gatekeeper/internal/github"
	"github.com/aac228/merge-gatekeeper/internal/github/mock"
	"github.com/aac228/merge-gatekeeper/internal/validators"
)

func stringPtr(str string) *string {
	return &str
}

func intPtr(v int) *int64 {
	i := int64(v)
	return &i
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestCreateValidator(t *testing.T) {
	tests := map[string]struct {
		c       github.Client
		opts    []Option
		want    validators.Validator
		wantErr bool
	}{
		"returns Validator when option is not empty": {
			c: &mock.Client{},
			opts: []Option{
				WithGitHubOwnerAndRepo("test-owner", "test-repo"),
				WithGitHubRef("sha"),
				WithSelfJob("job"),
				WithIgnoredJobs("job-01,job-02"),
			},
			want: &statusValidator{
				client:      &mock.Client{},
				owner:       "test-owner",
				repo:        "test-repo",
				ref:         "sha",
				selfJobName: "job",
				ignoredJobs: []string{"job-01", "job-02"},
			},
			wantErr: false,
		},
		"returns Validator when there are duplicate options": {
			c: &mock.Client{},
			opts: []Option{
				WithGitHubOwnerAndRepo("test", "test-repo"),
				WithGitHubRef("sha"),
				WithGitHubRef("sha-01"),
				WithSelfJob("job"),
				WithSelfJob("job-01"),
			},
			want: &statusValidator{
				client:      &mock.Client{},
				owner:       "test",
				repo:        "test-repo",
				ref:         "sha-01",
				selfJobName: "job-01",
			},
			wantErr: false,
		},
		"returns Validator when invalid string is provided for ignored jobs": {
			c: &mock.Client{},
			opts: []Option{
				WithGitHubOwnerAndRepo("test", "test-repo"),
				WithGitHubRef("sha"),
				WithGitHubRef("sha-01"),
				WithSelfJob("job"),
				WithSelfJob("job-01"),
				WithIgnoredJobs(","), // Malformed but handled
			},
			want: &statusValidator{
				client:      &mock.Client{},
				owner:       "test",
				repo:        "test-repo",
				ref:         "sha-01",
				selfJobName: "job-01",
				ignoredJobs: []string{}, // Not nil
			},
			wantErr: false,
		},
		"returns error when option is empty": {
			c:       &mock.Client{},
			want:    nil,
			wantErr: true,
		},
		"returns error when client is nil": {
			c: nil,
			opts: []Option{
				WithGitHubOwnerAndRepo("test", "test-repo"),
				WithGitHubRef("sha"),
				WithGitHubRef("sha-01"),
				WithSelfJob("job"),
				WithSelfJob("job-01"),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := CreateValidator(tt.c, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateValidator error = %v, wantErr: %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateValidator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestName(t *testing.T) {
	tests := map[string]struct {
		c    github.Client
		opts []Option
		want string
	}{
		"Name returns the correct job name which gets overridden": {
			c: &mock.Client{},
			opts: []Option{
				WithGitHubOwnerAndRepo("test-owner", "test-repo"),
				WithGitHubRef("sha"),
				WithSelfJob("job"),
				WithIgnoredJobs("job-01,job-02"),
			},
			want: "job",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := CreateValidator(tt.c, tt.opts...)
			if err != nil {
				t.Errorf("Unexpected error with CreateValidator: %v", err)
				return
			}
			if tt.want != got.Name() {
				t.Errorf("Job name didn't match, want: %s, got: %v", tt.want, got.Name())
			}
		})
	}
}

func Test_statusValidator_Validate(t *testing.T) {
	type test struct {
		selfJobName string
		ignoredJobs []string
		client      github.Client
		ctx         context.Context
		wantErr     bool
		wantErrStr  string
		wantStatus  validators.Status
	}
	tests := map[string]test{
		"returns succeeded status and nil when there is no job": {
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					return &github.WorkflowRuns{}, nil, nil
				},
			},
			wantErr: false,
			wantStatus: &status{
				succeeded:    true,
				totalJobs:    []string{},
				completeJobs: []string{},
				ignoredJobs:  []string{},
				errJobs:      []string{},
			},
		},
		"returns succeeded status and nil when there is one job, which is itself": {
			selfJobName: "self-job",
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					total := 1
					return &github.ListCheckRunsResults{
						Total: &total,
						CheckRuns: []*github.CheckRun{
							{
								Name:   stringPtr("self-job"),
								Status: stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Merge Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantStatus: &status{
				succeeded:    true,
				totalJobs:    []string{},
				completeJobs: []string{},
				ignoredJobs:  []string{},
				errJobs:      []string{},
			},
		},
		"returns failed status and nil when there is one job": {
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:   stringPtr("job"),
								Status: stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantStatus: &status{
				succeeded:    false,
				totalJobs:    []string{"Workflow / job"},
				completeJobs: []string{},
				ignoredJobs:  []string{},
				errJobs:      []string{},
			},
		},
		"returns error when there is a failed job": {
			selfJobName: "self-job",
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:       stringPtr("job-01"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:       stringPtr("job-02"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunTimedOutConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:   stringPtr("self-job"),
								Status: stringPtr(checkRunQueuedStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(2),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 2
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
							{
								Name:         stringPtr("Merge Workflow"),
								CheckSuiteID: intPtr(2),
							},
						},
					}, nil, nil
				},
			},
			wantErr: true,
			wantErrStr: (&status{
				totalJobs: []string{
					"Workflow / job-01", "Workflow / job-02",
				},
				completeJobs: []string{
					"Workflow / job-01",
				},
				errJobs: []string{
					"Workflow / job-02",
				},
				ignoredJobs: []string{},
			}).Detail(),
		},
		"returns error when there is a failed job with failure state": {
			selfJobName: "self-job",
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:       stringPtr("job-01"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:       stringPtr("job-02"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunFailedConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:   stringPtr("self-job"),
								Status: stringPtr(pendingState),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(2),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 2
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
							{
								Name:         stringPtr("Merge Workflow"),
								CheckSuiteID: intPtr(2),
							},
						},
					}, nil, nil
				},
			},
			wantErr: true,
			wantErrStr: (&status{
				totalJobs: []string{
					"Workflow / job-01", "Workflow / job-02",
				},
				completeJobs: []string{
					"Workflow / job-01",
				},
				errJobs: []string{
					"Workflow / job-02",
				},
				ignoredJobs: []string{},
			}).Detail(),
		},
		"returns failed status and nil when successful job count is less than total": {
			selfJobName: "self-job",
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:       stringPtr("job-01"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:   stringPtr("job-02"),
								Status: stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:   stringPtr("self-job"),
								Status: stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(2),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 2
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
							{
								Name:         stringPtr("Merge Workflow"),
								CheckSuiteID: intPtr(2),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantStatus: &status{
				succeeded: false,
				totalJobs: []string{
					"Workflow / job-01",
					"Workflow / job-02",
				},
				completeJobs: []string{
					"Workflow / job-01",
				},
				errJobs:     []string{},
				ignoredJobs: []string{},
			},
		},
		"returns succeeded status and nil when validation is success": {
			selfJobName: "self-job",
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:       stringPtr("job-01"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:       stringPtr("job-02"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(2),
								},
							},
							{
								Name:   stringPtr("self-job"),
								Status: stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(3),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 2
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow 1"),
								CheckSuiteID: intPtr(1),
							},
							{
								Name:         stringPtr("Workflow 2"),
								CheckSuiteID: intPtr(2),
							},
							{
								Name:         stringPtr("Merge Workflow"),
								CheckSuiteID: intPtr(3),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantStatus: &status{
				succeeded: true,
				totalJobs: []string{
					"Workflow 1 / job-01",
					"Workflow 2 / job-02",
				},
				completeJobs: []string{
					"Workflow 1 / job-01",
					"Workflow 2 / job-02",
				},
				errJobs:     []string{},
				ignoredJobs: []string{},
			},
		},
		"returns succeeded status and nil when only an ignored job is failing": {
			selfJobName: "self-job",
			ignoredJobs: []string{"job-02", "job-03"}, // String input here should be already TrimSpace'd
			client: &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:       stringPtr("job-01"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:       stringPtr("job-02"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunFailedConclusion),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(1),
								},
							},
							{
								Name:   stringPtr("self-job"),
								Status: stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{
									ID: intPtr(2),
								},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 2
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
							{
								Name:         stringPtr("Merge Workflow"),
								CheckSuiteID: intPtr(2),
							},
						},
					}, nil, nil
				},
			},
			wantErr: false,
			wantStatus: &status{
				succeeded:    true,
				totalJobs:    []string{"Workflow / job-01"},
				completeJobs: []string{"Workflow / job-01"},
				errJobs:      []string{},
				ignoredJobs:  []string{"job-02", "job-03"},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sv := &statusValidator{
				selfJobName: tt.selfJobName,
				ignoredJobs: tt.ignoredJobs,
				client:      tt.client,
			}
			got, err := sv.Validate(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("statusValidator.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err.Error() != tt.wantErrStr {
					t.Errorf("statusValidator.Validate() error.Error() = %s, wantErrStr %s", err.Error(), tt.wantErrStr)
				}
			}
			if !reflect.DeepEqual(got, tt.wantStatus) {
				t.Errorf("statusValidator.Validate() status = %v, want %v", got, tt.wantStatus)
			}
		})
	}
}

func Test_statusValidator_listStatuses(t *testing.T) {
	type fields struct {
		repo        string
		owner       string
		ref         string
		selfJobName string
		client      github.Client
	}
	type test struct {
		fields  fields
		ctx     context.Context
		wantErr bool
		want    []*ghaStatus
	}
	tests := map[string]test{
		"succeeds to get job statuses": func() test {
			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							// The first element here is the latest state.
							{
								Name:       stringPtr("job-02"),
								Status:     stringPtr(checkRunInProgressStatus),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-03"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunNeutralConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-04"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-05"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunFailedConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-06"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSkipConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: false,
				want: []*ghaStatus{
					{
						Job:      "job-02",
						State:    pendingState,
						Workflow: "Workflow",
					},
					{
						Job:      "job-03",
						State:    successState,
						Workflow: "Workflow",
					},
					{
						Job:      "job-04",
						State:    successState,
						Workflow: "Workflow",
					},
					{
						Job:      "job-05",
						State:    errorState,
						Workflow: "Workflow",
					},
				},
			}
		}(),
		"returns error when the ListCheckRunsForRef returns an error": func() test {
			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return nil, nil, errors.New("error")
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					return &github.WorkflowRuns{}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: true,
			}
		}(),
		"returns error when the ListCheckRunsForRef response is invalid": func() test {
			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					return &github.WorkflowRuns{}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: true,
			}
		}(),
		"returns nil when no error occurs": func() test {
			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					return &github.ListCheckRunsResults{
						CheckRuns: []*github.CheckRun{
							{
								Name:       stringPtr("job-02"),
								Status:     stringPtr(checkRunFailedConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-03"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunNeutralConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-04"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSuccessConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-05"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunFailedConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
							{
								Name:       stringPtr("job-06"),
								Status:     stringPtr(checkRunCompletedStatus),
								Conclusion: stringPtr(checkRunSkipConclusion),
								CheckSuite: &github.CheckSuite{ID: intPtr(1)},
							},
						},
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: false,
				want: []*ghaStatus{
					{
						Job:      "job-02",
						State:    pendingState,
						Workflow: "Workflow",
					},
					{
						Job:      "job-03",
						State:    successState,
						Workflow: "Workflow",
					},
					{
						Job:      "job-04",
						State:    successState,
						Workflow: "Workflow",
					},
					{
						Job:      "job-05",
						State:    errorState,
						Workflow: "Workflow",
					},
				},
			}
		}(),
		"succeeds to retrieve 100 statuses": func() test {
			num_statuses := 100
			statuses := make([]*github.RepoStatus, num_statuses)
			checkRuns := make([]*github.CheckRun, num_statuses)
			expectedGhaStatuses := make([]*ghaStatus, num_statuses)
			for i := 0; i < num_statuses; i++ {
				statuses[i] = &github.RepoStatus{
					Context: stringPtr(fmt.Sprintf("job-%d", i)),
					State:   stringPtr(successState),
				}

				checkRuns[i] = &github.CheckRun{
					Name:       stringPtr(fmt.Sprintf("job-%d", i)),
					Status:     stringPtr(checkRunCompletedStatus),
					Conclusion: stringPtr(checkRunNeutralConclusion),
					CheckSuite: &github.CheckSuite{ID: intPtr(1)},
				}

				expectedGhaStatuses[i] = &ghaStatus{
					Job:      fmt.Sprintf("job-%d", i),
					State:    successState,
					Workflow: "Workflow",
				}
			}

			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					l := len(checkRuns)
					return &github.ListCheckRunsResults{
						CheckRuns: checkRuns,
						Total:     &l,
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: false,
				want:    expectedGhaStatuses,
			}
		}(),
		"succeeds to retrieve 162 statuses": func() test {
			num_statuses := 162
			checkRuns := make([]*github.CheckRun, num_statuses)
			expectedGhaStatuses := make([]*ghaStatus, num_statuses)
			for i := 0; i < num_statuses; i++ {
				checkRuns[i] = &github.CheckRun{
					Name:       stringPtr(fmt.Sprintf("job-%d", i)),
					Status:     stringPtr(checkRunCompletedStatus),
					Conclusion: stringPtr(checkRunNeutralConclusion),
					CheckSuite: &github.CheckSuite{ID: intPtr(1)},
				}

				expectedGhaStatuses[i] = &ghaStatus{
					Job:      fmt.Sprintf("job-%d", i),
					State:    successState,
					Workflow: "Workflow",
				}
			}

			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					l := len(checkRuns)
					return &github.ListCheckRunsResults{
						CheckRuns: checkRuns,
						Total:     &l,
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: false,
				want:    expectedGhaStatuses,
			}
		}(),
		"succeeds to retrieve 587 check runs": func() test {
			num_statuses := 587
			checkRuns := make([]*github.CheckRun, num_statuses)
			expectedGhaStatuses := make([]*ghaStatus, num_statuses)
			for i := 0; i < num_statuses; i++ {
				checkRuns[i] = &github.CheckRun{
					Name:       stringPtr(fmt.Sprintf("job-%d", i)),
					Status:     stringPtr(checkRunCompletedStatus),
					Conclusion: stringPtr(checkRunNeutralConclusion),
					CheckSuite: &github.CheckSuite{ID: intPtr(1)},
				}

				expectedGhaStatuses[i] = &ghaStatus{
					Job:      fmt.Sprintf("job-%d", i),
					State:    successState,
					Workflow: "Workflow",
				}
			}

			c := &mock.Client{
				ListCheckRunsForRefFunc: func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
					l := len(checkRuns)
					return &github.ListCheckRunsResults{
						CheckRuns: checkRuns,
						Total:     &l,
					}, nil, nil
				},
				ListWorkflowRunsFunc: func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
					total := 1
					return &github.WorkflowRuns{
						TotalCount: &total,
						WorkflowRuns: []*github.WorkflowRun{
							{
								Name:         stringPtr("Workflow"),
								CheckSuiteID: intPtr(1),
							},
						},
					}, nil, nil
				},
			}
			return test{
				fields: fields{
					client:      c,
					selfJobName: "self-job",
					owner:       "test-owner",
					repo:        "test-repo",
					ref:         "main",
				},
				wantErr: false,
				want:    expectedGhaStatuses,
			}
		}(),
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sv := &statusValidator{
				repo:        tt.fields.repo,
				owner:       tt.fields.owner,
				ref:         tt.fields.ref,
				selfJobName: tt.fields.selfJobName,
				client:      tt.fields.client,
			}
			got, err := sv.listGhaStatuses(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("statusValidator.listStatuses() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got, want := len(got), len(tt.want); got != want {
				t.Errorf("statusValidator.listStatuses() length = %v, want %v", got, want)
			}
			for i := range tt.want {
				if !reflect.DeepEqual(got[i], tt.want[i]) {
					t.Errorf("statusValidator.listStatuses() - %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
