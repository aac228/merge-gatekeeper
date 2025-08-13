package mock

import (
	"context"

	"github.com/aac228/merge-gatekeeper/internal/github"
)

type Client struct {
	ListCheckRunsForRefFunc func(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error)
	ListWorkflowRunsFunc    func(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error)
}

func (c *Client) ListCheckRunsForRef(ctx context.Context, owner, repo, ref string, opts *github.ListCheckRunsOptions) (*github.ListCheckRunsResults, *github.Response, error) {
	return c.ListCheckRunsForRefFunc(ctx, owner, repo, ref, opts)
}

func (c *Client) ListWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
	return c.ListWorkflowRunsFunc(ctx, owner, repo, opts)
}

var (
	_ github.Client = &Client{}
)
