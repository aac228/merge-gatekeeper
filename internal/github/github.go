package github

import (
	"context"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

type (
	ListOptions             = github.ListOptions
	CombinedStatus          = github.CombinedStatus
	RepoStatus              = github.RepoStatus
	Response                = github.Response
	ListWorkflowRunsOptions = github.ListWorkflowRunsOptions
)

type (
	CheckRun             = github.CheckRun
	CheckSuite           = github.CheckSuite
	ListCheckRunsOptions = github.ListCheckRunsOptions
	ListCheckRunsResults = github.ListCheckRunsResults
	WorkflowRuns         = github.WorkflowRuns
	WorkflowRun          = github.WorkflowRun
)

type Client interface {
	ListCheckRunsForRef(ctx context.Context, owner, repo, ref string, opts *ListCheckRunsOptions) (*ListCheckRunsResults, *Response, error)
	ListWorkflowRuns(ctx context.Context, owner, repo string, opts *ListWorkflowRunsOptions) (*WorkflowRuns, *github.Response, error)
}

type client struct {
	ghc *github.Client
}

func NewClient(ctx context.Context, token string) Client {
	return &client{
		ghc: github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: token,
			},
		))),
	}
}

func (c *client) GetCombinedStatus(ctx context.Context, owner, repo, ref string, opts *ListOptions) (*CombinedStatus, *Response, error) {
	return c.ghc.Repositories.GetCombinedStatus(ctx, owner, repo, ref, opts)
}

func (c *client) ListCheckRunsForRef(ctx context.Context, owner, repo, ref string, opts *ListCheckRunsOptions) (*ListCheckRunsResults, *Response, error) {
	return c.ghc.Checks.ListCheckRunsForRef(ctx, owner, repo, ref, opts)
}

func (c *client) ListWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
	return c.ghc.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
}
